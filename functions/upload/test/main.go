package main

import (
	"context"
	"crypto/tls"
	"github.com/chrismarget-j/apstraami"
	"log"
	"net/http"
	"os"
)

func main() {
	keyLogWriter, err := os.OpenFile("/Users/cmarget/.tls.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}

	payload := apstraami.FetchAndExtractRequest{
		//Url:        "https://thisissostupid.s3.amazonaws.com/dummy.ova",
		//BucketName: "apstra-images-20221203153114834900000002",
		//Files:      map[string]string{
		//	"a/b/c/dummy-disk1.vmdk": "",
		//},

		Url:        "https://cdn.juniper.net/software/jafc/4.1.1/aos_server_4.1.1-287.ova?SM_USER=cmarget&__gda__=1670126455_91aee8f5ef2451ad331958b053072292",
		BucketName: "apstra-images-go20221204033847469800000002",
		Files: map[string]string{
			"aos_server_4.1.1-287-disk1.vmdk": "aos_server_4.1.1-287-disk1.vmdk",
		},

		HttpClient: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{KeyLogWriter: keyLogWriter}}},
	}

	faer, err := apstraami.FetchAndExtract(context.TODO(), payload)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(faer.Etags)
}
