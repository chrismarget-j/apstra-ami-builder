package toami

import (
	"encoding/json"
	"log"
)

//type Request struct {
//}

type Response struct {
}

func HandleRequest(request any) (*Response, error) {
	dump, _ := json.Marshal(request)
	log.Printf("request received: '%s'", string(dump))

	return &Response{}, nil
}
