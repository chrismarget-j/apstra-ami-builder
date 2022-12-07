package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstra-ami-builder/functions/upload"
)

func main() {
	lambda.Start(upload.HandleRequest)
}
