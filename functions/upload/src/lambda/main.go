package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	uploadx "github.com/chrismarget-j/apstraami/upload"
)

func main() {
	lambda.Start(uploadx.HandleRequest)
}
