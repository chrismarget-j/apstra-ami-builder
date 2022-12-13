package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstra-ami-builder/functions/amimaker"
)

func main() {
	lambda.Start(amimaker.HandleRequest)
}
