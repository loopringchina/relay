/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package gateway

import (
	"errors"
	"fmt"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/types"
	"net"
	"github.com/Loopring/relay/market"
	"github.com/Loopring/relay/ordermanager"
	"math/big"
	"github.com/ethereum/go-ethereum/common"
	"github.com/Loopring/relay/log"
	"github.com/ethereum/go-ethereum/rpc"
	"strings"
	"github.com/Loopring/relay/market/util"
)

func (*JsonrpcServiceImpl) Ping(val string, val2 int) (res string, err error) {
	fmt.Println(val)
	fmt.Println(val2)
	res = "pong for first connect, meaning server is OK"
	return
}

type PageResult struct {
	Data      []interface{}
	PageIndex int
	PageSize  int
	Total     int
}

type Depth struct {
	contractVersion string
	market          string
	Depth           AskBid
}

type AskBid struct {
	Buy  [][]string
	Sell [][]string
}

type CommonTokenRequest struct {
	contractVersion string
	owner           string
}

type OrderQuery struct {
	Status string
	PageIndex int
	PageSize  int
	ContractVersion string
	Owner           string
}

type DepthQuery struct {
	Length int
	ContractVersion string
	Market           string
}

type FillQuery struct {
	ContractVersion string
	Market           string
	Owner string
	OrderHash string
	RingHash string
	PageIndex int
	PageSize  int
}

type RingMinedQuery struct {
	ContractVersion string
	RingHash string
	PageIndex int
	PageSize  int
}

var RemoteAddrContextKey = "RemoteAddr"

type JsonrpcService interface {
	Start(port string)
	Stop()
}

type JsonrpcServiceImpl struct {
	port           string
	trendManager   market.TrendManager
	orderManager   ordermanager.OrderManager
	accountManager market.AccountManager
	ethForwarder *EthForwarder
}

func NewJsonrpcService(port string, trendManager market.TrendManager, orderManager ordermanager.OrderManager, accountManager market.AccountManager, ethForwarder *EthForwarder) *JsonrpcServiceImpl {
	l := &JsonrpcServiceImpl{}
	l.port = port
	l.trendManager = trendManager
	l.orderManager = orderManager
	l.accountManager = accountManager
	l.ethForwarder = ethForwarder
	return l
}

func (j *JsonrpcServiceImpl) Start() {
	handler := rpc.NewServer()
	if err := handler.RegisterName("loopring", &JsonrpcServiceImpl{}); err != nil {
		fmt.Println(err)
		return
	}
	if err := handler.RegisterName("eth", j.ethForwarder); err != nil {
		fmt.Println(err)
		return
	}
	var (
		listener net.Listener
		err      error
	)
	if listener, err = net.Listen("tcp", ":8083"); err != nil {
		return
	}
	go rpc.NewHTTPServer([]string{"*"}, handler).Serve(listener)
	log.Info(fmt.Sprintf("HTTP endpoint opened: http://%s", ":8083"))

	return
}

func (j *JsonrpcServiceImpl) SubmitOrder(order *types.OrderJsonRequest)(res string, err error) {
	fmt.Println(*order)
	err = HandleOrder(types.ToOrder(order))
	if err != nil {
		fmt.Println(err)
	}
	res = "SUBMIT_SUCCESS"
	return res, err
}

func (j *JsonrpcServiceImpl) GetOrders(query OrderQuery)(res dao.PageResult, err error) {
	orderQuery, pi, ps := convertFromQuery(query)
	res, err = j.orderManager.GetOrders(&orderQuery, pi, ps)
	return res, err
}

func (j *JsonrpcServiceImpl) GetDepth(query DepthQuery) (res Depth, err error) {

	mkt := strings.ToLower(query.Market)
	protocol := query.ContractVersion
	length := query.Length

	fmt.Println(query)

	if mkt == "" || protocol == "" || util.ContractVersionConfig[protocol] == "" {
		err = errors.New("market and correct contract version must be applied")
		return
	}

	if length <= 0 || length > 20 {
		length = 20
	}

	a, b := util.UnWrap(mkt)
	if util.SupportTokens[a] == "" || util.SupportMarket[b] == "" {
		err = errors.New("unsupported market type")
		return
	}

	empty := make([][]string, 0)
	for i := range empty {
		empty[i] = make([]string, 0)
	}
	askBid := AskBid{Buy: empty, Sell: empty}
	depth := Depth{contractVersion: util.ContractVersionConfig[protocol], market: mkt, Depth: askBid}

	//(TODO) 考虑到需要聚合的情况，所以每次取2倍的数据，先聚合完了再cut, 不是完美方案，后续再优化
	asks, askErr := j.orderManager.GetOrderBook(
		common.StringToAddress(util.ContractVersionConfig[protocol]),
		common.StringToAddress(a),
		common.StringToAddress(b), length*2)

	if askErr != nil {
		err = errors.New("get depth error , please refresh again")
		return
	}

	depth.Depth.Sell = calculateDepth(asks, length)

	bids, bidErr := j.orderManager.GetOrderBook(
		common.StringToAddress(util.ContractVersionConfig[protocol]),
		common.StringToAddress(b),
		common.StringToAddress(a), length*2)

	if bidErr != nil {
		err = errors.New("get depth error , please refresh again")
		return
	}

	depth.Depth.Buy = calculateDepth(bids, length)

	return depth, err
}

func (j *JsonrpcServiceImpl) GetFills(query FillQuery) (res dao.PageResult, err error) {
	fmt.Println(query)
	return j.orderManager.FillsPageQuery(fillQueryToMap(query))
}

func (j *JsonrpcServiceImpl) GetTicker(market string) (res []market.Ticker, err error) {
	res, err = j.trendManager.GetTicker()
	return
}

func (j *JsonrpcServiceImpl) GetTrend(market string) (res []market.Trend, err error) {
	res, err = j.trendManager.GetTrends(market)
	return
}

func (j *JsonrpcServiceImpl) GetRingMined(query RingMinedQuery) (res dao.PageResult, err error) {
	fmt.Println(query)
	return j.orderManager.RingMinedPageQuery(ringMinedQueryToMap(query))
}

func (j *JsonrpcServiceImpl) GetBalance(balanceQuery CommonTokenRequest) (res market.AccountJson, err error) {
	account := j.accountManager.GetBalance(balanceQuery.contractVersion, balanceQuery.owner)
	res = account.ToJsonObject(balanceQuery.contractVersion)
	return
}

func convertFromQuery(orderQuery OrderQuery) (query dao.Order, pageIndex int, pageSize int) {

	status := convertStatus(orderQuery.Status)
	query.Status = uint8(status)
	query.Owner = orderQuery.Owner
	query.Protocol = util.ContractVersionConfig[orderQuery.ContractVersion]
	pageIndex = orderQuery.PageIndex
	pageSize = orderQuery.PageSize
	return

}

func convertStatus(s string) types.OrderStatus {
	switch s {
		case "ORDER_NEW":
			return types.ORDER_NEW
		case "ORDER_PARTIAL":
			return types.ORDER_PARTIAL
		case "ORDER_FINISHED":
			return types.ORDER_FINISHED
		case "ORDER_CANCEL":
			return types.ORDER_CANCEL
		case "ORDER_CUTOFF":
			return types.ORDER_CUTOFF
	}
	return types.ORDER_UNKNOWN
}

func calculateDepth(states []types.OrderState, length int) [][]string {

	if len(states) == 0 {
		return [][]string{}
	}

	depth := make([][]string, 0)
	for i := range depth {
		depth[i] = make([]string, 0)
	}

	var tempSumAmountS, tempSumAmountB big.Int
	var lastPrice big.Rat

	for i, s := range states {

		if i == 0 {
			lastPrice = *s.RawOrder.Price
			tempSumAmountS = *s.RawOrder.AmountS
			tempSumAmountB = *s.RawOrder.AmountB
		} else {
			if lastPrice.Cmp(s.RawOrder.Price) != 0 {
				depth = append(depth, []string{tempSumAmountS.String(), tempSumAmountB.String()})
				tempSumAmountS.Set(big.NewInt(0))
				tempSumAmountB.Set(big.NewInt(0))
				lastPrice = *s.RawOrder.Price
			} else {
				tempSumAmountS.Add(&tempSumAmountS, s.RawOrder.AmountS)
				tempSumAmountB.Add(&tempSumAmountB, s.RawOrder.AmountB)
			}
		}
	}

	return depth[:length]
}

func fillQueryToMap(q FillQuery) (map[string]string, int, int) {
	rst := make(map[string]string)
	var pi, ps int
	if q.Market != "" {
		rst["market"] = q.Market
	}
	if q.PageIndex <= 0 {
		pi = 1
	} else {
		pi = q.PageIndex
	}
	if q.PageSize <= 0 || q.PageSize > 20 {
		ps = 20
	} else {
		ps = q.PageIndex
	}
	if q.ContractVersion != "" {
		rst["contract_version"] = util.ContractVersionConfig[q.ContractVersion]
	}
	if q.Owner != "" {
		rst["owner"] = q.Owner
	}
	if q.OrderHash != "" {
		rst["order_hash"] = q.OrderHash
	}
	if q.RingHash != "" {
		rst["ring_hash"] = q.RingHash
	}

	return rst, pi, ps
}


func ringMinedQueryToMap(q RingMinedQuery) (map[string]interface{}, int, int) {
	rst := make(map[string]interface{})
	var pi, ps int
	if q.PageIndex <= 0 {
		pi = 1
	} else {
		pi = q.PageIndex
	}
	if q.PageSize <= 0 || q.PageSize > 20 {
		ps = 20
	} else {
		ps = q.PageIndex
	}
	if q.ContractVersion != "" {
		rst["contract_version"] = util.ContractVersionConfig[q.ContractVersion]
	}
	if q.RingHash != "" {
		rst["ring_hash"] = q.RingHash
	}

	return rst, pi, ps
}