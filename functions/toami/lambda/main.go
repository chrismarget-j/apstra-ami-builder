package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstraami/toami"
	"log"
)

func main() {
	lambda.Start(toami.HandleRequest)

	log.Println("hello world")
}
