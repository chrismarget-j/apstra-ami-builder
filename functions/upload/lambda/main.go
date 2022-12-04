package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstraami"
)

func main() {
	lambda.Start(apstraami.HandleRequest)
}
