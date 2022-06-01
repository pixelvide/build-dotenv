package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

func getSecret(secretName string) string {
	profile := "pixelvide"
	if _, ok := os.LookupEnv("DOTENV_AWS_PROFILE"); ok {
		profile = os.Getenv("DOTENV_AWS_PROFILE")
	}

	fmt.Println(profile)

	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: profile,
	})

	if err != nil {
		panic(err)
	}

	// Create a Secrets Manager client
	svc := secretsmanager.New(sess, &aws.Config{})

	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	}

	result, err := svc.GetSecretValue(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeDecryptionFailure:
				// Secrets Manager can't decrypt the protected secret text using the provided KMS key.
				fmt.Println(secretsmanager.ErrCodeDecryptionFailure, aerr.Error())

			case secretsmanager.ErrCodeInternalServiceError:
				// An error occurred on the server side.
				fmt.Println(secretsmanager.ErrCodeInternalServiceError, aerr.Error())

			case secretsmanager.ErrCodeInvalidParameterException:
				// You provided an invalid value for a parameter.
				fmt.Println(secretsmanager.ErrCodeInvalidParameterException, aerr.Error())

			case secretsmanager.ErrCodeInvalidRequestException:
				// You provided a parameter value that is not valid for the current state of the resource.
				fmt.Println(secretsmanager.ErrCodeInvalidRequestException, aerr.Error())

			case secretsmanager.ErrCodeResourceNotFoundException:
				// We can't find the resource that you asked for.
				fmt.Println(secretsmanager.ErrCodeResourceNotFoundException, aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}

		panic(err)
	}

	// Decrypts secret using the associated KMS CMK.
	// Depending on whether the secret is a string or binary, one of these fields will be populated.
	if result.SecretString != nil {
		return *result.SecretString
	} else {
		decodedBinarySecretBytes := make([]byte, base64.StdEncoding.DecodedLen(len(result.SecretBinary)))
		len, err := base64.StdEncoding.Decode(decodedBinarySecretBytes, result.SecretBinary)
		if err != nil {
			fmt.Println("Base64 Decode Error:", err)
			panic(err)
		}
		return string(decodedBinarySecretBytes[:len])
	}

}

func main() {
	envData := make(map[string]string)

	targetFilePath, ok := os.LookupEnv("DOTENV_FILE_PATH")
	if !ok {
		targetFilePath = ".env"
	}

	if _, ok := os.LookupEnv("CI_PROJECT_DIR"); ok {
		targetFilePath = os.Getenv("CI_PROJECT_DIR") + "/" + targetFilePath
	}

	awsSecretConfigs := strings.Split(os.Getenv("AWS_SECRET_CONFIGS"), ",")

	// check if file not exists then create a blank file
	if _, err := os.Stat(targetFilePath); os.IsNotExist(err) {
		if _, err := os.Create(targetFilePath); err != nil {
			panic(err)
		}
	}

	// Read file
	file, err := os.Open(targetFilePath)

	//handle errors while opening
	if err != nil {
		panic(err)
	}

	fileScanner := bufio.NewScanner(file) // the file is inside the local directory
	// read line by line
	for fileScanner.Scan() {
		line := fileScanner.Text()
		if !strings.HasPrefix(line, "#") {
			keyValue := strings.Split(line, "=")
			keyValue = append(keyValue, "")

			key := keyValue[0]
			value := strings.Join(keyValue[1:], "")

			envData[key] = value
		}
	}

	// handle first encountered error while reading
	if err := fileScanner.Err(); err != nil {
		panic(err)
	}

	for x := 0; x < len(awsSecretConfigs); x++ {
		secretString := getSecret(awsSecretConfigs[x])
		sec := map[string]string{}
		if err := json.Unmarshal([]byte(secretString), &sec); err != nil {
			panic(err)
		}
		for key, val := range sec {
			envData[key] = val
		}
	}

	result := make([]string, 0)
	for key, val := range envData {
		if key != "" {
			if strings.HasPrefix(val, "/") {
				result = append(result, string(key)+"="+val+"")
			} else {
				result = append(result, string(key)+"=\""+val+"\"")
			}
		}
	}

	file, err = os.Create(targetFilePath)
	writer := bufio.NewWriter(file)
	for _, line := range result {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			log.Fatalf("Got error while writing to a file. Err: %s", err.Error())
		}
	}
	writer.Flush()

	fmt.Println(".env generated successfully...!")
}
