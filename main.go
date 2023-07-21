package main

import (
	"flag"
	"fmt"
	"log"
	b "math/big"
	"sort"
	"strconv"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/gin-gonic/gin"
)

func main() {
	var port string
	flag.StringVar(&port, "port", ":8099", "Specify a port")
	flag.Parse()

	r := gin.Default()
	// 使用查询参数解析 URL 参数
	r.GET("/penalty", func(c *gin.Context) {
		// 获取查询参数值
		miner := c.Query("miner")
		if miner == "" {
			c.String(400, "please specify a miner")
		}
		mid, err := address.NewFromString(miner)
		if err != nil {
			c.String(400, err.Error())
		}

		allSectors, _ := strconv.ParseBool(c.DefaultQuery("all", "0"))

		// 往后/往前 推多少天
		offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

		data, err := Compute(mid, allSectors, abi.ChainEpoch(offset*2880))
		if err != nil {
			log.Printf("%v\n", err)
			c.String(500, "Internal Server Error")
		}
		c.String(200, data)

	})
	r.Run(port)
}

func Compute(mid address.Address, allSectors bool, offset abi.ChainEpoch) (string, error) {

	type daliyData struct {
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
		}
	}

	var onChainInfo []*miner.SectorOnChainInfo
	if !allSectors {
		onChainInfo, err = lapi.StateMinerActiveSectors(ctx, mid, types.EmptyTSK)
		if err != nil {
			return "", err
		}
	} else {
		onChainInfo, err = lapi.StateMinerSectors(ctx, mid, nil, types.EmptyTSK)
		if err != nil {
			return "", err
		}
	}

	sumData := make(map[string]*daliyData, 540)
	for _, info := range onChainInfo {
		date := heightToTime(int64(info.Expiration))
		live := (tsk.Height() + offset - info.Activation) / 2880

		var penalty abi.TokenAmount
		if live >= 140 {
			penalty = big.Add(info.ExpectedStoragePledge, big.Mul(info.ExpectedDayReward, big.NewInt(int64(70))))
		} else {
			penalty = big.Add(info.ExpectedStoragePledge, big.Mul(info.ExpectedDayReward, big.NewInt(int64(live/2))))
		}

		if data, ok := sumData[date]; ok {
			data.info[uint64(info.SectorNumber)] = info.InitialPledge
			data.penalty = big.Add(data.penalty, penalty)
		} else {
			sumData[date] = &daliyData{penalty: penalty, info: make(map[uint64]abi.TokenAmount)}

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

		outData += fmt.Sprintf("%v,%v,%v,%v,%v,%v\n", date, mid, seLen, float64(minerInfo.SectorSize*abi.SectorSize(seLen))/(1<<40), new(b.Rat).SetFrac(daliyPledge.Int, b.NewInt(1e18)).FloatString(10), new(b.Rat).SetFrac(data.penalty.Int, b.NewInt(1e18)).FloatString(10))

		sectors_sum += seLen
		power += minerInfo.SectorSize * abi.SectorSize(seLen)
		pledge = big.Add(pledge, daliyPledge)
		penalty = big.Add(penalty, data.penalty)
	}
	// 汇总数据
	outData += fmt.Sprintf(",,%v,%v,%v,%v\n", sectors_sum, float64(power)/(1<<40), new(b.Rat).SetFrac(pledge.Int, b.NewInt(1e18)).FloatString(10), new(b.Rat).SetFrac(penalty.Int, b.NewInt(1e18)).FloatString(10))

	return outData, nil
}

func heightToTime(height int64) string {
	timestamp := bootstrapTime + height*30
	// 使用 time.Unix() 将时间戳转换为日期
	dateTime := time.Unix(timestamp, 0)
	// 将日期转换为指定格式的字符串
	dateString := dateTime.Format("2006-01-02")
	return dateString
}
