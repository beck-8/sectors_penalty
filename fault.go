package main

import (
	"net/http"
	"strconv"

	"github.com/filecoin-project/go-state-types/big"
	m "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/gin-gonic/gin"
)

func faultFee(c *gin.Context) {
	jsonOut, _ := strconv.ParseBool(c.DefaultQuery("json", "0"))

	tsk, err := lapi.ChainHead(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code: http.StatusInternalServerError,
			Msg:  "ChainHead err",
		})
		return
	}
	rewardEstimate, networkQAPowerEstimate, err := GetSmoothing(tsk)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code: http.StatusInternalServerError,
			Msg:  "GetSmoothing err",
		})
		return
	}
	fee := m.ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate, big.NewInt(32*1024*1024*1024), 10108)
	if jsonOut {
		c.JSON(http.StatusOK, APIResponse{
			Code: http.StatusOK,
			Msg:  "OK",
			Data: fee,
		})
	} else {
		c.String(200, fee.String())
	}

}
