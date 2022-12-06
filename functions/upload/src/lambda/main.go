package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstraami/upload"
)

func main() {
	lambda.Start(upload.HandleRequest)
}
