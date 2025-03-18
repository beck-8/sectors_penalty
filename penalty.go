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
	m "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	s "github.com/filecoin-project/go-state-types/builtin/v16/util/smoothing"
	gststore "github.com/filecoin-project/go-state-types/store"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/gin-gonic/gin"
)

// https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0098.md#specification
var (
	TERMINATION_LIFETIME_CAP = int64(140)

	TERM_FEE_PLEDGE_MULTIPLE_NUM   = big.NewInt(85)
	TERM_FEE_PLEDGE_MULTIPLE_DENOM = big.NewInt(1000)

	TERM_FEE_MIN_PLEDGE_MULTIPLE_NUM   = big.NewInt(2)
	TERM_FEE_MIN_PLEDGE_MULTIPLE_DENOM = big.NewInt(100)

	TERM_FEE_MAX_FAULT_FEE_MULTIPLE_NUM   = big.NewInt(105)
	TERM_FEE_MAX_FAULT_FEE_MULTIPLE_DENOM = big.NewInt(100)

	// todo: 高度未确定
	nv25Height = abi.ChainEpoch(4863600)
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

	rewardEstimate, networkQAPowerEstimate, err := GetSmoothing(tsk)
	if err != nil {
		return "", err
	}

	sumData := make(map[string]*dailyData, 540)
	for _, info := range onChainInfo {
		// date := heightToTime(int64(info.Expiration) + int64(deadlines[uint64(info.SectorNumber)]*60))
		// 上述已丢弃，弃用，应该是nv15丢弃的
		date := heightToTime(int64(m.QuantSpecForDeadline(m.NewDeadlineInfo(cd.PeriodStart, uint64(deadlines[uint64(info.SectorNumber)]), 0)).QuantizeUp(info.Expiration)))

		var penalty abi.TokenAmount

		if tsk.Height() < nv25Height {
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
		} else {

			penalty = PledgePenaltyForTermination(info.InitialPledge, int64(tsk.Height()+offset-info.Activation), FaultFee(minerInfo.SectorSize, info, rewardEstimate, networkQAPowerEstimate))
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

// copy from builtin-actors
func PledgePenaltyForTermination(initial_pledge abi.TokenAmount, sector_age int64, fault_fee abi.TokenAmount) abi.TokenAmount {
	simple_termination_fee := big.Div(big.Mul(initial_pledge, TERM_FEE_PLEDGE_MULTIPLE_NUM), TERM_FEE_PLEDGE_MULTIPLE_DENOM)
	duration_termination_fee := big.Div(big.Mul(big.NewInt(sector_age), simple_termination_fee), big.NewInt(TERMINATION_LIFETIME_CAP*2880))
	base_termination_fee := big.Min(simple_termination_fee, duration_termination_fee)

	minimum_fee_abs := big.Div(big.Mul(initial_pledge, TERM_FEE_MIN_PLEDGE_MULTIPLE_NUM), TERM_FEE_MIN_PLEDGE_MULTIPLE_DENOM)
	minimum_fee_ff := big.Div(big.Mul(fault_fee, TERM_FEE_MAX_FAULT_FEE_MULTIPLE_NUM), TERM_FEE_MAX_FAULT_FEE_MULTIPLE_DENOM)
	minimum_fee := big.Max(minimum_fee_abs, minimum_fee_ff)

	return big.Max(base_termination_fee, minimum_fee)
}

// pub const CONTINUED_FAULT_PROJECTION_PERIOD: ChainEpoch = (EPOCHS_IN_DAY * CONTINUED_FAULT_FACTOR_NUM) / CONTINUED_FAULT_FACTOR_DENOM;
// 3.51 * dayward
func FaultFee(size abi.SectorSize, info *m.SectorOnChainInfo, rewardEstimate s.FilterEstimate, networkQAPowerEstimate s.FilterEstimate) abi.TokenAmount {
	qaPower := m.QAPowerForSector(size, info)
	fee := m.ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate, qaPower, 10080)
	return fee
}

func GetSmoothing(ts *types.TipSet) (s.FilterEstimate, s.FilterEstimate, error) {
	bs := blockstore.NewAPIBlockstore(lapi)
	ctxStore := gststore.WrapBlockStore(ctx, bs)

	powerActor, err := lapi.StateGetActor(ctx, power.Address, ts.Key())
	if err != nil {
		return s.FilterEstimate{}, s.FilterEstimate{}, err
	}

	powerState, err := power.Load(ctxStore, powerActor)
	if err != nil {
		return s.FilterEstimate{}, s.FilterEstimate{}, err
	}

	rewardActor, err := lapi.StateGetActor(ctx, reward.Address, ts.Key())
	if err != nil {
		return s.FilterEstimate{}, s.FilterEstimate{}, err
	}

	rewardState, err := reward.Load(ctxStore, rewardActor)
	if err != nil {
		return s.FilterEstimate{}, s.FilterEstimate{}, err
	}

	networkQAPower, err := powerState.TotalPowerSmoothed()
	if err != nil {
		return s.FilterEstimate{}, s.FilterEstimate{}, err
	}

	thisEpochRewardSmoothed, err := rewardState.(interface {
		ThisEpochRewardSmoothed() (builtin.FilterEstimate, error)
	}).ThisEpochRewardSmoothed()
	if err != nil {
		return s.FilterEstimate{}, s.FilterEstimate{}, err
	}

	rewardEstimate := s.FilterEstimate{
		PositionEstimate: thisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: thisEpochRewardSmoothed.VelocityEstimate,
	}
	networkQAPowerEstimate := s.FilterEstimate{
		PositionEstimate: networkQAPower.PositionEstimate,
		VelocityEstimate: networkQAPower.VelocityEstimate,
	}
	return rewardEstimate, networkQAPowerEstimate, nil
}
