package main

import (
	"bytes"
	"fmt"
	"math/big"
	"net/http"
	"strconv"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/gin-gonic/gin"
	"github.com/olekukonko/tablewriter"
)

type dailyFee struct {
	Qap32G   float64 `json:"qap_32g"`
	Qap1T    float64 `json:"qap_1t"`
	Qap100T  float64 `json:"qap_100t"`
	Qap1024T float64 `json:"qap_1024t"`
}

type spFee struct {
	DailyFee float64 `json:"daily_fee"`
	TotalFee float64 `json:"total_fee"`
}

var (
	// 5.56e-15 / 32GiB = 5.56e-15 / (32 * 2^30) = 5.56e-15 / 34,359,738,368 ≈ 1.61817e-25
	// k = 5.56e-15

	DAILY_FEE_CIRCULATING_SUPPLY_QAP_MULTIPLIER_NUM   = big.NewInt(161817)
	DAILY_FEE_CIRCULATING_SUPPLY_QAP_MULTIPLIER_DENOM = new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil) // 10^30
)

func getDailyFee(c *gin.Context) {
	jsonOut, _ := strconv.ParseBool(c.DefaultQuery("json", "0"))

	data, err := computeDailyFee(jsonOut)
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

// FIP-100
func computeDailyFee(jsonOut bool) (interface{}, error) {

	circulatingSupply, err := lapi.StateVMCirculatingSupplyInternal(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	d := dailyFee{
		Qap32G:   CalculateQAPFee(circulatingSupply, big.NewInt(32<<30)),
		Qap1T:    CalculateQAPFee(circulatingSupply, big.NewInt(1<<40)),
		Qap100T:  CalculateQAPFee(circulatingSupply, big.NewInt(100<<40)),
		Qap1024T: CalculateQAPFee(circulatingSupply, big.NewInt(1024<<40)),
	}

	if jsonOut {
		return d, nil
	}

	head, err := lapi.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	// Create buffer and table writer
	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("Chain Height: %d\n", head.Height()))
	buf.WriteString(fmt.Sprintf("Chain Timestamp: %d\n", head.MinTimestamp()))
	buf.WriteString(fmt.Sprintf("FilCirculating: %s FIL\n", big.NewInt(0).Div(circulatingSupply.FilCirculating.Int, big.NewInt(1e18))))

	table := tablewriter.NewWriter(buf)
	// Set table title
	table.SetCaption(false, "Daily Fee Details")
	// Set table header
	table.SetHeader([]string{"Size(QAP)", "Daily Fee(FIL)", "210 Fee(FIL)", "540 Fee(FIL)"})

	// Add rows
	table.Append([]string{"32G", fmt.Sprintf("%.12f", d.Qap32G), fmt.Sprintf("%.12f", d.Qap32G*210), fmt.Sprintf("%.12f", d.Qap32G*540)})
	table.Append([]string{"1T", fmt.Sprintf("%.12f", d.Qap1T), fmt.Sprintf("%.12f", d.Qap1T*210), fmt.Sprintf("%.12f", d.Qap1T*540)})
	table.Append([]string{"100T", fmt.Sprintf("%.12f", d.Qap100T), fmt.Sprintf("%.12f", d.Qap100T*210), fmt.Sprintf("%.12f", d.Qap100T*540)})
	table.Append([]string{"1024T", fmt.Sprintf("%.12f", d.Qap1024T), fmt.Sprintf("%.12f", d.Qap1024T*210), fmt.Sprintf("%.12f", d.Qap1024T*540)})

	// Configure table style
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(true)

	// Render table
	table.Render()
	return buf.String(), nil
}

// calculateQAPFee calculates the daily fee for a given QAP size in bytes
func CalculateQAPFee(circulatingSupply api.CirculatingSupply, qapBytes *big.Int) float64 {
	// DAILY_FEE_CIRCULATING_SUPPLY_QAP_MULTIPLIER_NUM * circulatingSupply.FilCirculating.Int * 32 * 2^30 / DAILY_FEE_CIRCULATING_SUPPLY_QAP_MULTIPLIER_DENOM
	qap := new(big.Rat).SetInt(DAILY_FEE_CIRCULATING_SUPPLY_QAP_MULTIPLIER_NUM)
	qap.Mul(qap, new(big.Rat).SetInt(circulatingSupply.FilCirculating.Int))
	qap.Mul(qap, new(big.Rat).SetInt(qapBytes))
	qap.Quo(qap, new(big.Rat).SetInt(DAILY_FEE_CIRCULATING_SUPPLY_QAP_MULTIPLIER_DENOM))
	qap.Quo(qap, new(big.Rat).SetInt(big.NewInt(1e18)))

	qapFloat, _ := qap.Float64()
	return qapFloat
}

func getSpDailyFee(c *gin.Context) {
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

	data, err := computeSpDailyFee(mid, jsonOut)
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

func computeSpDailyFee(mid address.Address, jsonOut bool) (interface{}, error) {
	d := spFee{}

	tsk, err := lapi.ChainHead(ctx)
	if err != nil {
		return "", err
	}

	deadlines, err := lapi.StateMinerDeadlines(ctx, mid, types.EmptyTSK)
	if err != nil {
		return "", err
	}
	for _, deadline := range deadlines {
		if deadline.DailyFee.NilOrZero() {
			continue
		}
		fee, _ := new(big.Rat).Quo(new(big.Rat).SetInt(deadline.DailyFee.Int), new(big.Rat).SetInt(big.NewInt(1e18))).Float64()
		d.DailyFee += fee
	}

	liveSectors := make(map[uint64]bool)
	for i := 0; i < 48; i++ {
		partitions, err := lapi.StateMinerPartitions(ctx, mid, uint64(i), types.EmptyTSK)
		if err != nil {
			return "", err
		}
		for _, part := range partitions {
			liveCount, err := part.LiveSectors.Count()
			if err != nil {
				return "", err
			}
			liveSector, err := part.LiveSectors.AllMap(liveCount)
			if err != nil {
				return "", err
			}
			for k, v := range liveSector {
				liveSectors[k] = v
			}
		}
	}
	var onChainInfo []*miner.SectorOnChainInfo
	tmp, err := lapi.StateMinerSectors(ctx, mid, nil, types.EmptyTSK)
	if err != nil {
		return "", err
	}
	for _, v := range tmp {
		if liveSectors[uint64(v.SectorNumber)] {
			onChainInfo = append(onChainInfo, v)
		}
	}

	for _, info := range onChainInfo {
		if info.DailyFee.NilOrZero() {
			continue
		}
		// 简单计算，实际会有一天内的误差
		// Simple calculation, there will actually be an error of one day
		days := (float64(info.Expiration) - float64(tsk.Height())) / 2880
		fee, _ := new(big.Rat).SetInt64(0).Quo(new(big.Rat).SetInt(info.DailyFee.Int), new(big.Rat).SetInt(big.NewInt(1e18))).Float64()
		d.TotalFee += days * fee
	}
	if jsonOut {
		return d, nil
	}
	buf := new(bytes.Buffer)
	buf.WriteString(fmt.Sprintf("Chain Height: %d\n", tsk.Height()))
	buf.WriteString(fmt.Sprintf("Chain Timestamp: %d\n", tsk.MinTimestamp()))
	buf.WriteString(fmt.Sprintf("Miner: %s\n", mid.String()))
	buf.WriteString(fmt.Sprintf("Sectors: %d\n", len(onChainInfo)))
	buf.WriteString(fmt.Sprintf("Daily Fee: %.12f FIL\n", d.DailyFee))
	buf.WriteString(fmt.Sprintf("Total Fee: %.12f FIL\n", d.TotalFee))
	buf.WriteString("Ps: Daily Fee * day != Total Fee, because the expiration time of the sector is different")

	return buf.String(), nil

}
