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

package ordermanager

import (
	"fmt"
	"github.com/Loopring/relay/config"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/market/util"
	"github.com/Loopring/relay/marketcap"
	"github.com/Loopring/relay/types"
	"github.com/Loopring/relay/usermanager"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"sync"
)

type OrderManager interface {
	Start()
	Stop()
	MinerOrders(protocol, tokenS, tokenB common.Address, length int, filterOrderhashs []common.Hash) []*types.OrderState
	GetOrderBook(protocol, tokenS, tokenB common.Address, length int) ([]types.OrderState, error)
	GetOrders(query map[string]interface{}, pageIndex, pageSize int) (dao.PageResult, error)
	GetOrderByHash(hash common.Hash) (*types.OrderState, error)
	UpdateBroadcastTimeByHash(hash common.Hash, bt int) error
	FillsPageQuery(query map[string]interface{}, pageIndex, pageSize int) (dao.PageResult, error)
	RingMinedPageQuery(query map[string]interface{}, pageIndex, pageSize int) (dao.PageResult, error)
	IsOrderCutoff(owner common.Address, createTime *big.Int) bool
	IsOrderFullFinished(state *types.OrderState) bool
}

type OrderManagerImpl struct {
	options            config.OrderManagerOptions
	commonOpts         *config.CommonOptions
	rds                dao.RdsService
	lock               sync.RWMutex
	processor          *forkProcessor
	accessor           *ethaccessor.EthNodeAccessor
	um                 usermanager.UserManager
	mc                 *marketcap.MarketCapProvider
	cutoffCache        *CutoffCache
	newOrderWatcher    *eventemitter.Watcher
	ringMinedWatcher   *eventemitter.Watcher
	fillOrderWatcher   *eventemitter.Watcher
	cancelOrderWatcher *eventemitter.Watcher
	cutoffOrderWatcher *eventemitter.Watcher
	forkWatcher        *eventemitter.Watcher
}

func NewOrderManager(options config.OrderManagerOptions,
	commonOpts *config.CommonOptions,
	rds dao.RdsService,
	userManager usermanager.UserManager,
	accessor *ethaccessor.EthNodeAccessor,
	market *marketcap.MarketCapProvider) *OrderManagerImpl {

	om := &OrderManagerImpl{}
	om.options = options
	om.commonOpts = commonOpts
	om.rds = rds
	om.processor = newForkProcess(om.rds, accessor)
	om.accessor = accessor
	om.um = userManager
	om.mc = market
	om.cutoffCache = NewCutoffCache(rds)
	om.accessor = accessor

	return om
}

// Start start orderbook as a service
func (om *OrderManagerImpl) Start() {
	om.newOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleGatewayOrder}
	om.ringMinedWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleRingMined}
	om.fillOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderFilled}
	om.cancelOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderCancelled}
	om.cutoffOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderCutoff}
	om.forkWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleFork}

	eventemitter.On(eventemitter.OrderManagerGatewayNewOrder, om.newOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorRingMined, om.ringMinedWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorFill, om.fillOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorCancel, om.cancelOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorCutoff, om.cutoffOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerFork, om.forkWatcher)
}

func (om *OrderManagerImpl) Stop() {
	om.lock.Lock()
	defer om.lock.Unlock()

	eventemitter.Un(eventemitter.OrderManagerGatewayNewOrder, om.newOrderWatcher)
	eventemitter.Un(eventemitter.OrderManagerExtractorRingMined, om.ringMinedWatcher)
	eventemitter.Un(eventemitter.OrderManagerExtractorFill, om.fillOrderWatcher)
	eventemitter.Un(eventemitter.OrderManagerExtractorCancel, om.cancelOrderWatcher)
	eventemitter.Un(eventemitter.OrderManagerExtractorCutoff, om.cutoffOrderWatcher)
	eventemitter.Un(eventemitter.OrderManagerFork, om.forkWatcher)
}

func (om *OrderManagerImpl) handleFork(input eventemitter.EventData) error {
	om.Stop()

	if err := om.processor.fork(input.(*types.ForkedEvent)); err != nil {
		log.Errorf("order manager,handle fork error:%s", err.Error())
	}

	om.Start()

	return nil
}

// 来自ipfs的新订单
// 所有来自ipfs的订单都是新订单
func (om *OrderManagerImpl) handleGatewayOrder(input eventemitter.EventData) error {
	om.lock.Lock()
	defer om.lock.Unlock()

	state := input.(*types.OrderState)
	state.Status = types.ORDER_NEW
	state.DealtAmountS = big.NewInt(0)
	state.DealtAmountB = big.NewInt(0)
	state.CancelledAmountS = big.NewInt(0)
	state.CancelledAmountB = big.NewInt(0)

	log.Debugf("order manager,handle gateway order,order.hash:%s amountS:%s", state.RawOrder.Hash.Hex(), state.RawOrder.AmountS.String())

	// order already exist in dao/order
	if _, err := om.rds.GetOrderByHash(state.RawOrder.Hash); err == nil {
		return nil
	}

	// get order cancelled or filled amount from chain
	if cancelOrFilledAmount, err := om.accessor.GetCancelledOrFilled(state.RawOrder.Protocol, state.RawOrder.Hash, "latest"); err != nil {
		return fmt.Errorf("order manager,handle gateway order,order %s getCancelledOrFilled error:%s", state.RawOrder.Hash.Hex(), err.Error())
	} else {
		state.CancelledAmountS = cancelOrFilledAmount
	}

	// check order finished status
	finished := om.IsOrderFullFinished(state)
	state.SettleFinishedStatus(finished)

	// check allowance and balance
	var markBlockNumber int64 = 0
	if state.Status != types.ORDER_FINISHED {
		spender, _ := om.accessor.GetSenderAddress(state.RawOrder.Protocol)
		req := generateErc20Req(state, spender)
		if err := om.accessor.BatchErc20BalanceAndAllowance([]*ethaccessor.BatchErc20Req{req}); err != nil {
			return fmt.Errorf("order manager,geteway new order,batchErc20BalanceAndAllowance error:%s", err.Error())
		}
		if req.AllowanceErr != nil || req.BalanceErr != nil {
			return fmt.Errorf("order manager,gateway new order,order %s ab ")
		}
		calculateAmountS(state, req)
		if ok := om.IsFundInsufficient(state); ok {
			markBlockNumber = state.UpdatedBlock.Int64() + int64(om.options.AccountPeriod)
		}
	}

	model := &dao.Order{}
	model.MinerBlockMark = markBlockNumber
	model.Market, _ = util.WrapMarketByAddress(state.RawOrder.TokenB.Hex(), state.RawOrder.TokenS.Hex())
	model.ConvertDown(state)

	return om.rds.Add(model)
}

func (om *OrderManagerImpl) handleRingMined(input eventemitter.EventData) error {
	event := input.(*types.RingMinedEvent)

	model := &dao.RingMinedEvent{}
	if err := model.ConvertDown(event); err != nil {
		return err
	}
	if err := om.rds.Add(model); err != nil {
		log.Debugf("order manager,handle ringmined event,event %s has already exist", event.RingIndex.String())
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderFilled(input eventemitter.EventData) error {
	event := input.(*types.OrderFilledEvent)

	// save event
	_, err := om.rds.FindFillEventByRinghashAndOrderhash(event.Ringhash, event.OrderHash)
	if err == nil {
		return fmt.Errorf("order manager,handle order filled event,fill already exist ringIndex:%s orderHash:", event.RingIndex.String(), event.OrderHash.Hex())
	}

	newFillModel := &dao.FillEvent{}
	if err := newFillModel.ConvertDown(event); err != nil {
		log.Debugf("order manager,handle order filled event error:order %s convert down failed", event.OrderHash.Hex())
		return err
	}
	if err := om.rds.Add(newFillModel); err != nil {
		log.Debugf("order manager,handle order filled event error:order %s insert faild", event.OrderHash.Hex())
		return err
	}

	// get rds.Order and types.OrderState
	state := &types.OrderState{UpdatedBlock: event.Blocknumber}
	model, err := om.rds.GetOrderByHash(event.OrderHash)
	if err != nil {
		return err
	}
	if err := model.ConvertUp(state); err != nil {
		return err
	}

	// judge order status
	if state.Status == types.ORDER_CUTOFF || state.Status == types.ORDER_FINISHED || state.Status == types.ORDER_UNKNOWN {
		return fmt.Errorf("order manager,handle order filled event error:order %s status is %d ", state.RawOrder.Hash.Hex(), state.Status)
	}

	// calculate dealt amount
	state.UpdatedBlock = event.Blocknumber
	state.DealtAmountS = new(big.Int).Add(state.DealtAmountS, event.AmountS)
	state.DealtAmountB = new(big.Int).Add(state.DealtAmountB, event.AmountB)
	log.Debugf("order manager,handle order filled event orderhash:%s,dealAmountS:%s,dealtAmountB:%s", state.RawOrder.Hash.Hex(), state.DealtAmountS.String(), state.DealtAmountB.String())

	// update order status
	finished := om.IsOrderFullFinished(state)
	state.SettleFinishedStatus(finished)

	// update rds.Order
	if err := model.ConvertDown(state); err != nil {
		log.Errorf(err.Error())
		return err
	}
	if err := om.rds.UpdateOrderWhileFill(state.RawOrder.Hash, state.Status, state.DealtAmountS, state.DealtAmountB, state.UpdatedBlock); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderCancelled(input eventemitter.EventData) error {
	event := input.(*types.OrderCancelledEvent)

	// save event
	_, err := om.rds.FindCancelEvent(event.OrderHash, event.TxHash)
	if err == nil {
		return fmt.Errorf("order manager,handle order cancelled event error:event %s have already exist", event.OrderHash)
	}
	newCancelEventModel := &dao.CancelEvent{}
	if err := newCancelEventModel.ConvertDown(event); err != nil {
		return err
	}
	if err := om.rds.Add(newCancelEventModel); err != nil {
		return err
	}

	// get rds.Order and types.OrderState
	state := &types.OrderState{}
	model, err := om.rds.GetOrderByHash(event.OrderHash)
	if err != nil {
		return err
	}
	if err := model.ConvertUp(state); err != nil {
		return err
	}

	// judge status
	if state.Status == types.ORDER_CUTOFF || state.Status == types.ORDER_FINISHED || state.Status == types.ORDER_UNKNOWN {
		return fmt.Errorf("order manager,handle order cancelled event error:order %s status is %d ", event.OrderHash.Hex(), state.Status)
	}

	// calculate remainAmount
	if state.RawOrder.BuyNoMoreThanAmountB {
		state.CancelledAmountB = new(big.Int).Add(state.CancelledAmountB, event.AmountCancelled)
		log.Debugf("order manager,handle order cancelled event,order:%s cancelled amountb:%s", state.RawOrder.Hash.Hex(), state.CancelledAmountB.String())
	} else {
		state.CancelledAmountS = new(big.Int).Add(state.CancelledAmountS, event.AmountCancelled)
		log.Debugf("order manager,handle order cancelled event,order:%s cancelled amounts:%s", state.RawOrder.Hash.Hex(), state.CancelledAmountS.String())
	}

	// update order status
	finished := om.IsOrderFullFinished(state)
	state.SettleFinishedStatus(finished)

	// update rds.Order
	if err := model.ConvertDown(state); err != nil {
		return err
	}
	if err := om.rds.UpdateOrderWhileCancel(state.RawOrder.Hash, state.Status, state.CancelledAmountS, state.CancelledAmountB, state.UpdatedBlock); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderCutoff(input eventemitter.EventData) error {
	event := input.(*types.CutoffEvent)

	if err := om.rds.SettleOrdersCutoffStatus(event.Owner, event.Cutoff); err != nil {
		log.Debugf("order manager,handle cutoff event,%s", err.Error())
	}
	if err := om.cutoffCache.Add(event); err != nil {
		return err
	}

	log.Debugf("order manager,handle cutoff event, owner:%s, cutoffTimestamp:%s", event.Owner.Hex(), event.Cutoff.String())
	return nil
}

func (om *OrderManagerImpl) IsFundInsufficient(state *types.OrderState) bool {
	price := om.mc.GetMarketCap(state.RawOrder.TokenS)
	amount := new(big.Rat).SetInt(state.AvailableAmountS)
	value := new(big.Rat).Mul(price, amount)

	if value.Cmp(big.NewRat(1, 1)) > 0 {
		return false
	}

	return true
}

func (om *OrderManagerImpl) IsOrderFullFinished(state *types.OrderState) bool {
	var valueOfRemainAmount *big.Rat

	if state.RawOrder.BuyNoMoreThanAmountB {
		cancelOrFilledAmountB := new(big.Int).Add(state.DealtAmountB, state.CancelledAmountB)
		remainAmountB := new(big.Int).Sub(state.RawOrder.AmountB, cancelOrFilledAmountB)
		ratRemainAmountB := new(big.Rat).SetInt(remainAmountB)
		price := om.mc.GetMarketCap(state.RawOrder.TokenB)
		valueOfRemainAmount = new(big.Rat).Mul(price, ratRemainAmountB)
	} else {
		cancelOrFilledAmountS := new(big.Int).Add(state.DealtAmountS, state.CancelledAmountS)
		remainAmountS := new(big.Int).Sub(state.RawOrder.AmountS, cancelOrFilledAmountS)
		ratRemainAmountS := new(big.Rat).SetInt(remainAmountS)
		price := om.mc.GetMarketCap(state.RawOrder.TokenS)
		valueOfRemainAmount = new(big.Rat).Mul(price, ratRemainAmountS)
	}

	// todo: get compare number from config
	if valueOfRemainAmount.Cmp(big.NewRat(1, 1)) > 0 {
		return false
	}

	return true
}

func (om *OrderManagerImpl) MinerOrders(protocol, tokenS, tokenB common.Address, length int, filterOrderhashs []common.Hash) []*types.OrderState {
	var (
		list            []*types.OrderState
		modelList       []*dao.Order
		currentBlock    *dao.Block
		markBlockNumber *big.Int
		err             error
		orderhashstrs   []string
		filterStatus    = []types.OrderStatus{types.ORDER_FINISHED, types.ORDER_CUTOFF, types.ORDER_CANCEL}
	)

	for _, v := range filterOrderhashs {
		orderhashstrs = append(orderhashstrs, v.Hex())
	}

	// 从数据库中获取最近处理的block，数据库为空表示程序从未运行过，这个时候所有的order.markBlockNumber都为0
	if currentBlock, err = om.rds.FindLatestBlock(); err == nil {
		var b types.Block
		currentBlock.ConvertUp(&b)
		markBlockNumber = b.BlockNumber
	} else {
		markBlockNumber = big.NewInt(0)
	}

	// 标记miner提供的劣质订单
	if err = om.rds.MarkMinerOrders(orderhashstrs, markBlockNumber.Int64()); err != nil {
		log.Debugf("order manager,provide orders for miner error:%s", err.Error())
	}

	// 从数据库获取订单
	markBlockNumber = new(big.Int).Sub(markBlockNumber, big.NewInt(int64(om.options.BlockPeriod)))
	if modelList, err = om.rds.GetOrdersForMiner(protocol.Hex(), tokenS.Hex(), tokenB.Hex(), length, filterStatus, markBlockNumber.Int64()); err != nil {
		return list
	}
	var listBeforeCheckAccount []*types.OrderState
	for _, v := range modelList {
		state := &types.OrderState{}
		v.ConvertUp(state)
		if !om.um.InWhiteList(state.RawOrder.TokenS) {
			listBeforeCheckAccount = append(listBeforeCheckAccount, state)
		}
	}

	// 批量查询订单账户的余额及授权
	var erc20ReqList []*ethaccessor.BatchErc20Req
	for _, v := range listBeforeCheckAccount {
		spender, _ := om.accessor.GetSenderAddress(v.RawOrder.Protocol)
		batchReq := generateErc20Req(v, spender)
		erc20ReqList = append(erc20ReqList, batchReq)
	}
	if len(erc20ReqList) == 0 {
		return list
	}
	if err := om.accessor.BatchErc20BalanceAndAllowance(erc20ReqList); err != nil {
		log.Debugf("order manager,miner orders,batchErc20BalanceAndAllowance error:%s", err.Error())
		return list
	}

	// 根据余额及授权过滤订单
	var accountMarkList []string
	for idx, req := range erc20ReqList {
		v := listBeforeCheckAccount[idx]
		if req.BalanceErr != nil || req.AllowanceErr != nil {
			continue
		}
		calculateAmountS(v, req)
		if om.IsFundInsufficient(v) {
			accountMarkList = append(accountMarkList, v.RawOrder.Hash.Hex())
		} else {
			list = append(list, v)
		}
	}

	// 标记余额/授权不足订单
	accountForbiddenBlockMark := currentBlock.BlockNumber + int64(om.options.AccountPeriod)
	if err = om.rds.MarkMinerOrders(accountMarkList, accountForbiddenBlockMark); err != nil {
		log.Debugf("order manager,provide orders for miner error:%s", err.Error())
	}

	return list
}

func (om *OrderManagerImpl) GetOrderBook(protocol, tokenS, tokenB common.Address, length int) ([]types.OrderState, error) {
	var list []types.OrderState
	models, err := om.rds.GetOrderBook(protocol, tokenS, tokenB, length)
	if err != nil {
		return list, err
	}

	for _, v := range models {
		var state types.OrderState
		if err := v.ConvertUp(&state); err != nil {
			continue
		}
		list = append(list, state)
	}

	return list, nil
}

func (om *OrderManagerImpl) GetOrders(query map[string]interface{}, pageIndex, pageSize int) (dao.PageResult, error) {
	var (
		pageRes dao.PageResult
	)
	tmp, err := om.rds.OrderPageQuery(query, pageIndex, pageSize)

	if err != nil {
		return pageRes, err
	}
	pageRes.PageIndex = tmp.PageIndex
	pageRes.PageSize = tmp.PageSize
	pageRes.Total = tmp.Total

	for _, v := range tmp.Data {
		var state types.OrderState
		model := v.(dao.Order)
		if err := model.ConvertUp(&state); err != nil {
			continue
		}
		pageRes.Data = append(pageRes.Data, state)
	}

	return pageRes, nil
}

func (om *OrderManagerImpl) GetOrderByHash(hash common.Hash) (orderState *types.OrderState, err error) {
	var result types.OrderState
	order, err := om.rds.GetOrderByHash(hash)
	if err != nil {
		return nil, err
	}

	if err := order.ConvertUp(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (om *OrderManagerImpl) UpdateBroadcastTimeByHash(hash common.Hash, bt int) error {
	return om.rds.UpdateBroadcastTimeByHash(hash.Str(), bt)
}

func (om *OrderManagerImpl) FillsPageQuery(query map[string]interface{}, pageIndex, pageSize int) (result dao.PageResult, err error) {
	return om.rds.FillsPageQuery(query, pageIndex, pageSize)
}

func (om *OrderManagerImpl) RingMinedPageQuery(query map[string]interface{}, pageIndex, pageSize int) (result dao.PageResult, err error) {
	return om.rds.RingMinedPageQuery(query, pageIndex, pageSize)
}

func (om *OrderManagerImpl) IsOrderCutoff(owner common.Address, createTime *big.Int) bool {
	return om.cutoffCache.IsOrderCutoff(owner, createTime)
}
