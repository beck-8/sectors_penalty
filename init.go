package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
)

var LotusNodeAddr = string([]byte{104, 116, 116, 112, 58, 47, 47, 49, 50, 56, 46, 49, 51, 54, 46, 49, 53, 55, 46, 49, 54, 52, 58, 54, 49, 50, 51, 52, 47, 114, 112, 99, 47, 118, 49})

var lapi api.FullNodeStruct
var ctx = context.Background()

var bootstrapTime = int64(1598306400)

func init() {
	rpc := os.Getenv("FULLNODE_RPC")
	if rpc != "" {
		LotusNodeAddr = rpc
	}
	//headers := http.Header{"Authorization": []string{"Bearer " + authToken}}
	headers := http.Header{
		"content-type": []string{"application/json"},
		//"Authorization": []string{"Bearer eyJh..............1gNY"},
	}
	closer, err := jsonrpc.NewMergeClient(context.Background(), LotusNodeAddr, "Filecoin", []interface{}{&lapi.Internal, &lapi.CommonStruct.Internal}, headers)
	if err != nil {
		log.Panicf("connecting with lotus failed: %s", err)
	}
	defer closer()

}
