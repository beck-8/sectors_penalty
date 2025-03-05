package main

import (
	"context"
	"log"
	"os"

	"github.com/filecoin-project/lotus/api/v0api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
)

var LotusApi = string([]byte{47, 105, 112, 52, 47, 49, 50, 56, 46, 49, 51, 54, 46, 49, 53, 55, 46, 49, 54, 52, 47, 116, 99, 112, 47, 54, 49, 50, 51, 52, 47, 104, 116, 116, 112})

var lapi v0api.FullNode
var ctx = context.Background()

var bootstrapTime = int64(1598306400)

var dateFormat = "2006-01-02"

func init() {

	if api := os.Getenv("FULLNODE_API_INFO"); api == "" {
		err := os.Setenv("FULLNODE_API_INFO", LotusApi)
		if err != nil {
			log.Panicln(err)
		}
	}
	if e := os.Getenv("DATE_FORMAT"); e != "" {
		dateFormat = e
	}

	var err error
	lapi, _, err = lcli.GetFullNodeAPI(cli.NewContext(&cli.App{}, nil, nil))
	if err != nil {
		log.Panicln(err)
	}
}
