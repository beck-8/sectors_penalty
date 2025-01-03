package main

import (
	"fmt"
	b "math/big"
	"net/http"
	"strconv"

	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/gin-gonic/gin"
)

func vestedFunds(c *gin.Context) {
	// 获取查询参数值
	miner := c.Query("miner")
	if miner == "" {
		c.JSON(http.StatusBadRequest, APIResponse{
			Code: http.StatusBadRequest,
			Msg:  "please specify a miner",
		})
		return
	}
	mid, err := address.NewFromString(miner)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Code: http.StatusBadRequest,
			Msg:  err.Error(),
		})
		return
	}
	jsonOut, _ := strconv.ParseBool(c.DefaultQuery("json", "0"))

	data, err := getVested(mid, jsonOut)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code: http.StatusInternalServerError,
			Msg:  err.Error(),
		})
		return
	}

	if jsonOut {
		c.JSON(http.StatusOK, APIResponse{
			Code: http.StatusOK,
			Msg:  "OK",
			Data: data,
		})
	} else {
		c.String(200, data.(string))
	}

}

func getVested(mid address.Address, jsonOut bool) (interface{}, error) {
	mact, err := lapi.StateGetActor(ctx, mid, types.EmptyTSK)
	if err != nil {
		return "", err
	}
	stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(lapi))
	mas, err := miner.Load(stor, mact)
	if err != nil {
		return "", err
	}
	lockedFund, err := mas.LockedFunds()
	if err != nil {
		return "", err
	}

	type dayData struct {
		Date        string `json:"date"`
		VestedFunds string `json:"vested_funds"`
	}
	dayDatas := make([]*dayData, 0)
	var data string
	data += fmt.Sprintln("Date,VestedFunds(FIL)")

	startEpoch := getTodayHeight()
	oldVested := abi.NewTokenAmount(0)
	for lockedFund.VestingFunds.GreaterThan(big.NewInt(0)) {
		// 从明天0点高度开始
		startEpoch += 2880
		vested, err := mas.VestedFunds(startEpoch)
		if err != nil {
			return "", err
		}

		dayVested := big.Sub(vested, oldVested)
		oldVested = vested

		lockedFund.VestingFunds = big.Sub(lockedFund.VestingFunds, dayVested)
		structData := &dayData{
			Date:        heightToTime(int64(startEpoch - 1)),
			VestedFunds: new(b.Rat).SetFrac(dayVested.Int, b.NewInt(1e18)).FloatString(10),
		}
		dayDatas = append(dayDatas, structData)
		data += fmt.Sprintf("%v,%v\n", structData.Date, structData.VestedFunds)
	}
	if jsonOut {
		return dayDatas, nil
	}
	return data, nil
}

func getTodayHeight() abi.ChainEpoch {
	// 获取当前时间
	currentTime := time.Now()

	// 设置当前时间的小时、分钟、秒和纳秒部分为0，得到今日0点的时间
	todayZeroTime := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, currentTime.Location())

	// 将今日0点的时间转换为时间戳（秒级）
	timestamp := todayZeroTime.Unix()
	// 今日0点的高度
	return abi.ChainEpoch((timestamp - bootstrapTime) / 30)
}
