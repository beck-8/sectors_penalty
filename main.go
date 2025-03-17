package main

import (
	"flag"

	"github.com/gin-gonic/gin"
)

func main() {
	var port string
	flag.StringVar(&port, "port", ":8099", "Specify a port")
	flag.Parse()

	r := gin.Default()
	// 使用查询参数解析 URL 参数
	r.GET("/penalty", penalty)
	r.GET("/vested", vestedFunds)
	r.GET("/dailyfee", getDailyFee)
	r.GET("/spdailyfee", getSpDailyFee)
	r.Run(port)
}
