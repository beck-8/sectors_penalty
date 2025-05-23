package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	var port string
	var showVersion bool

	flag.StringVar(&port, "port", ":8099", "Specify a port")
	flag.BoolVar(&showVersion, "v", false, "Display version information")
	flag.Parse()

	if showVersion {
		fmt.Println("Version:", UserVersion())
		os.Exit(0)
	}

	r := gin.Default()
	// 使用查询参数解析 URL 参数
	r.GET("/penalty", penalty)
	r.GET("/vested", vestedFunds)
	r.GET("/dailyfee", getDailyFee)
	r.GET("/spdailyfee", getSpDailyFee)
	r.GET("/faultfee", faultFee)
	r.Run(port)
}
