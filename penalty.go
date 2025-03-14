package main

import (
	"fmt"
	"log"
	b "math/big"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	m "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	Code  int         `json:"code"`
	Level int         `json:"level"`
	Msg   string      `json:"msg"`
	Data  interface{} `json:"data"`
}

func penalty(c *gin.Context) {
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

	allSectors, _ := strconv.ParseBool(c.DefaultQuery("all", "0"))

	// 往后/往前 推多少天
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	jsonOut, _ := strconv.ParseBool(c.DefaultQuery("json", "0"))

	data, err := Compute(mid, allSectors, abi.ChainEpoch(offset*2880), jsonOut)
	if err != nil {
		log.Printf("%v\n", err)
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

func Compute(mid address.Address, allSectors bool, offset abi.ChainEpoch, jsonOut bool) (interface{}, error) {

	type dailyData struct {
		penalty abi.TokenAmount
		info    map[uint64]abi.TokenAmount
	}

	tsk, err := lapi.ChainHead(ctx)
	if err != nil {
		return "", err
	}

	minerInfo, err := lapi.StateMinerInfo(ctx, mid, types.EmptyTSK)
	if err != nil {
		return "", err
	}

	cd, err := lapi.StateMinerProvingDeadline(ctx, mid, tsk.Key())
	if err != nil {
		return "", err
	}
	//todo: pre-allocation
	liveSectors := make(map[uint64]bool)
	deadlines := make(map[uint64]int)
	for i := 0; i < 48; i++ {
		partitions, err := lapi.StateMinerPartitions(ctx, mid, uint64(i), types.EmptyTSK)
		if err != nil {
			return "", err
		}
		for _, part := range partitions {
			count, err := part.AllSectors.Count()
			if err != nil {
				return "", err
			}
			sectors, err := part.AllSectors.All(count)
			if err != nil {
				return "", err
			}
			for _, sec := range sectors {
				deadlines[sec] = i
			}

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
	if allSectors {
		onChainInfo, err = lapi.StateMinerSectors(ctx, mid, nil, types.EmptyTSK)
		if err != nil {
			return "", err
		}
	} else {
		tmp, err := lapi.StateMinerSectors(ctx, mid, nil, types.EmptyTSK)
		if err != nil {
			return "", err
		}
		for _, v := range tmp {
			if liveSectors[uint64(v.SectorNumber)] {
				onChainInfo = append(onChainInfo, v)
			}
		}
	}

	sumData := make(map[string]*dailyData, 540)
	for _, info := range onChainInfo {
		// date := heightToTime(int64(info.Expiration) + int64(deadlines[uint64(info.SectorNumber)]*60))
		// 上述已丢弃，弃用，应该是nv15丢弃的
		date := heightToTime(int64(m.QuantSpecForDeadline(m.NewDeadlineInfo(cd.PeriodStart, uint64(deadlines[uint64(info.SectorNumber)]), 0)).QuantizeUp(info.Expiration)))

		var penalty abi.TokenAmount

		// https://github.com/filecoin-project/builtin-actors/blob/54236ae89880bf4aa89b0dba6d9060c3fd2aacee/actors/miner/src/monies.rs#L202
		// ctrl c ctrl v 的，所以没有遵循golang的命名规范
		lifetime_cap := int64(140 * 2880)
		var capped_sector_age int64
		if sector_age := int64(tsk.Height()+offset) - int64(info.PowerBaseEpoch); lifetime_cap < sector_age {
			capped_sector_age = lifetime_cap
		} else {
			capped_sector_age = sector_age
		}
		if capped_sector_age < 0 {
			capped_sector_age = 0
		}
		expected_reward := big.Mul(info.ExpectedDayReward, big.NewInt(capped_sector_age))

		var relevant_replaced_age int64
		if replaced_sector_age := int64(info.PowerBaseEpoch) - int64(info.Activation); replaced_sector_age < lifetime_cap-capped_sector_age {
			relevant_replaced_age = replaced_sector_age
		} else {
			relevant_replaced_age = lifetime_cap - capped_sector_age
		}
		expected_reward = big.Add(expected_reward, big.Mul(info.ReplacedDayReward, big.NewInt(relevant_replaced_age)))
		expected_reward = big.Div(expected_reward, big.NewInt(2))

		penalty = big.Add(info.ExpectedStoragePledge, big.Div(expected_reward, big.NewInt(2880)))

		// 说明用户把offset设置了很大的负数，这个时候罚金就是ExpectedStoragePledge
		// 这样处理后，t = tsk.Height()+offset，t在上次续期时间之后是准确的；t在扇区激活-上次续期时间之间是不太准确的；t在扇区激活之前是准确的。
		// |----|--bad--|----|
		if tsk.Height()+offset < info.Activation {
			penalty = info.ExpectedStoragePledge
		}

		if data, ok := sumData[date]; ok {
			data.info[uint64(info.SectorNumber)] = info.InitialPledge
			data.penalty = big.Add(data.penalty, penalty)
		} else {
			sumData[date] = &dailyData{penalty: penalty, info: make(map[uint64]abi.TokenAmount)}

			sumData[date].info[uint64(info.SectorNumber)] = info.InitialPledge

		}
	}

	// 将 map 中的键值对提取到切片中
	var sortedKeys []string
	for key := range sumData {
		sortedKeys = append(sortedKeys, key)
	}
	// 对切片进行排序（按日期键的规则）
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i] < sortedKeys[j]
	})

	type dayData struct {
		Date        string          `json:"date"`
		Mid         address.Address `json:"mid"`
		Sectors_sum int             `json:"sectors_sum"`
		Power       float64         `json:"power"`
		Pledge      string          `json:"pledge"`
		Penalty     string          `json:"penalty"`
	}
	dayDatas := make([]*dayData, 0)
	outData := ""
	// 表头
	outData += fmt.Sprintln("date,mid,sectors_sum,power(TiB),pledge,penalty")

	sectors_sum := 0
	power := abi.SectorSize(0)
	pledge := abi.NewTokenAmount(0)
	penalty := abi.NewTokenAmount(0)

	for _, date := range sortedKeys {
		data := sumData[date]
		daliyPledge := abi.NewTokenAmount(0)
		seLen := len(data.info)
		for _, v := range data.info {
			daliyPledge = big.Add(daliyPledge, v)
		}
		structData := &dayData{
			Date:        date,
			Mid:         mid,
			Sectors_sum: seLen,
			Power:       float64(minerInfo.SectorSize*abi.SectorSize(seLen)) / (1 << 40),
			Pledge:      new(b.Rat).SetFrac(daliyPledge.Int, b.NewInt(1e18)).FloatString(10),
			Penalty:     new(b.Rat).SetFrac(data.penalty.Int, b.NewInt(1e18)).FloatString(10),
		}
		dayDatas = append(dayDatas, structData)
		outData += fmt.Sprintf("%v,%v,%v,%v,%v,%v\n", date, mid, seLen, structData.Power, structData.Pledge, structData.Penalty)

		sectors_sum += seLen
		power += minerInfo.SectorSize * abi.SectorSize(seLen)
		pledge = big.Add(pledge, daliyPledge)
		penalty = big.Add(penalty, data.penalty)
	}
	// 汇总数据
	outData += fmt.Sprintf(",,%v,%v,%v,%v\n", sectors_sum, float64(power)/(1<<40), new(b.Rat).SetFrac(pledge.Int, b.NewInt(1e18)).FloatString(10), new(b.Rat).SetFrac(penalty.Int, b.NewInt(1e18)).FloatString(10))

	if jsonOut {
		return dayDatas, nil
	}
	return outData, nil
}

func heightToTime(height int64) string {
	timestamp := bootstrapTime + height*30
	// 使用 time.Unix() 将时间戳转换为日期
	dateTime := time.Unix(timestamp, 0)
	// 将日期转换为指定格式的字符串
	dateString := dateTime.Format(dateFormat)
	return dateString
}
