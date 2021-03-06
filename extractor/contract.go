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

package extractor

import (
	"fmt"
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/market/util"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

// 这里无需考虑版本问题，对解析来说，不接受版本升级带来数据结构变化的可能性
func (l *ExtractorServiceImpl) loadContract() {
	l.events = make(map[common.Hash]EventData)
	l.methods = make(map[string]MethodData)
	l.protocols = make(map[common.Address]string)

	l.loadProtocolAddress()
	l.loadErc20Contract()
	l.loadWethContract()
	l.loadProtocolContract()
	l.loadTokenRegisterContract()
	l.loadRingHashRegisteredContract()
	l.loadTokenTransferDelegateProtocol()
}

func (l *ExtractorServiceImpl) loadProtocolAddress() {
	for _, v := range util.AllTokens {
		l.protocols[v.Protocol] = v.Symbol
		log.Debugf("extractor,contract protocol %s->%s", v.Symbol, v.Protocol.Hex())
	}

	for _, v := range l.accessor.ProtocolAddresses {
		protocolSymbol := "loopring"
		delegateSymbol := "transfer_delegate"
		ringhashRegisterSymbol := "ringhash_register"
		tokenRegisterSymbol := "token_register"

		l.protocols[v.ContractAddress] = protocolSymbol
		l.protocols[v.TokenRegistryAddress] = tokenRegisterSymbol
		l.protocols[v.RinghashRegistryAddress] = ringhashRegisterSymbol
		l.protocols[v.DelegateAddress] = delegateSymbol

		log.Debugf("extractor,contract protocol %s->%s", protocolSymbol, v.ContractAddress.Hex())
		log.Debugf("extractor,contract protocol %s->%s", tokenRegisterSymbol, v.TokenRegistryAddress.Hex())
		log.Debugf("extractor,contract protocol %s->%s", ringhashRegisterSymbol, v.RinghashRegistryAddress.Hex())
		log.Debugf("extractor,contract protocol %s->%s", delegateSymbol, v.DelegateAddress.Hex())
	}
}

type EventData struct {
	Event           interface{}
	ContractAddress string // 某个合约具体地址
	TxHash          string // transaction hash
	CAbi            *abi.ABI
	Id              common.Hash
	Name            string
	BlockNumber     *big.Int
	Time            *big.Int
	Topics          []string
}

type MethodData struct {
	Method          interface{}
	ContractAddress string // 某个合约具体地址
	From            string
	To              string
	TxHash          string // transaction hash
	CAbi            *abi.ABI
	Id              string
	Name            string
	BlockNumber     *big.Int
	Time            *big.Int
	Value           *big.Int
	Input           string
	LogAmount       int
	Gas             *big.Int
	GasPrice        *big.Int
}

func (m *MethodData) IsValid() error {
	if m.LogAmount < 1 {
		return fmt.Errorf("method %s transaction logs == 0", m.Name)
	}
	return nil
}

const (
	RINGMINED_EVT_NAME           = "RingMined"
	CANCEL_EVT_NAME              = "OrderCancelled"
	CUTOFF_EVT_NAME              = "CutoffTimestampChanged"
	TRANSFER_EVT_NAME            = "Transfer"
	APPROVAL_EVT_NAME            = "Approval"
	TOKENREGISTERED_EVT_NAME     = "TokenRegistered"
	TOKENUNREGISTERED_EVT_NAME   = "TokenUnregistered"
	RINGHASHREGISTERED_EVT_NAME  = "RinghashSubmitted"
	ADDRESSAUTHORIZED_EVT_NAME   = "AddressAuthorized"
	ADDRESSDEAUTHORIZED_EVT_NAME = "AddressDeauthorized"

	SUBMITRING_METHOD_NAME          = "submitRing"
	CANCELORDER_METHOD_NAME         = "cancelOrder"
	SUBMITRINGHASH_METHOD_NAME      = "submitRinghash"
	BATCHSUBMITRINGHASH_METHOD_NAME = "batchSubmitRinghash"

	WETH_DEPOSIT_METHOD_NAME    = "deposit"
	WETH_WITHDRAWAL_METHOD_NAME = "withdraw"
)

func newEventData(event *abi.Event, cabi *abi.ABI) EventData {
	var c EventData

	c.Id = event.Id()
	c.Name = event.Name
	c.CAbi = cabi

	return c
}

func newMethodData(method *abi.Method, cabi *abi.ABI) MethodData {
	var c MethodData

	c.Id = common.ToHex(method.Id())
	c.Name = method.Name
	c.CAbi = cabi

	return c
}

func (l *ExtractorServiceImpl) loadProtocolContract() {
	for name, event := range l.accessor.ProtocolImplAbi.Events {
		if name != RINGMINED_EVT_NAME && name != CANCEL_EVT_NAME && name != CUTOFF_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, l.accessor.ProtocolImplAbi)

		switch contract.Name {
		case RINGMINED_EVT_NAME:
			contract.Event = &ethaccessor.RingMinedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleRingMinedEvent}
		case CANCEL_EVT_NAME:
			contract.Event = &ethaccessor.OrderCancelledEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleOrderCancelledEvent}
		case CUTOFF_EVT_NAME:
			contract.Event = &ethaccessor.CutoffTimestampChangedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleCutoffTimestampEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		l.events[contract.Id] = contract
		log.Debugf("extracotr,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range l.accessor.ProtocolImplAbi.Methods {
		if name != SUBMITRING_METHOD_NAME && name != CANCELORDER_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, l.accessor.ProtocolImplAbi)
		watcher := &eventemitter.Watcher{}

		switch contract.Name {
		case SUBMITRING_METHOD_NAME:
			contract.Method = &ethaccessor.SubmitRingMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleSubmitRingMethod}
		case CANCELORDER_METHOD_NAME:
			contract.Method = &ethaccessor.CancelOrderMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleCancelOrderMethod}
		}

		eventemitter.On(contract.Id, watcher)
		l.methods[contract.Id] = contract
		log.Debugf("extracotr,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (l *ExtractorServiceImpl) loadErc20Contract() {
	for name, event := range l.accessor.Erc20Abi.Events {
		if name != TRANSFER_EVT_NAME && name != APPROVAL_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, l.accessor.Erc20Abi)

		switch contract.Name {
		case TRANSFER_EVT_NAME:
			contract.Event = &ethaccessor.TransferEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleTransferEvent}
		case APPROVAL_EVT_NAME:
			contract.Event = &ethaccessor.ApprovalEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleApprovalEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		l.events[contract.Id] = contract
		log.Debugf("extracotr,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}

func (l *ExtractorServiceImpl) loadWethContract() {
	for name, method := range l.accessor.WethAbi.Methods {
		if name != WETH_DEPOSIT_METHOD_NAME && name != WETH_WITHDRAWAL_METHOD_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newMethodData(&method, l.accessor.WethAbi)

		switch contract.Name {
		case WETH_DEPOSIT_METHOD_NAME:
			// weth deposit without any inputs,use transaction.value as input
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleWethDepositMethod}
		case WETH_WITHDRAWAL_METHOD_NAME:
			contract.Method = &ethaccessor.WethWithdrawalMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleWethWithdrawalMethod}
		}

		eventemitter.On(contract.Id, watcher)
		l.methods[contract.Id] = contract
		log.Debugf("extracotr,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (l *ExtractorServiceImpl) loadTokenRegisterContract() {
	for name, event := range l.accessor.TokenRegistryAbi.Events {
		if name != TOKENREGISTERED_EVT_NAME && name != TOKENUNREGISTERED_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, l.accessor.TokenRegistryAbi)

		switch contract.Name {
		case TOKENREGISTERED_EVT_NAME:
			contract.Event = &ethaccessor.TokenRegisteredEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleTokenRegisteredEvent}
		case TOKENUNREGISTERED_EVT_NAME:
			contract.Event = &ethaccessor.TokenUnRegisteredEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleTokenUnRegisteredEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		l.events[contract.Id] = contract
		log.Debugf("extracotr,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}

func (l *ExtractorServiceImpl) loadRingHashRegisteredContract() {
	for name, event := range l.accessor.RinghashRegistryAbi.Events {
		if name != RINGHASHREGISTERED_EVT_NAME {
			continue
		}

		contract := newEventData(&event, l.accessor.RinghashRegistryAbi)
		contract.Event = &ethaccessor.RingHashSubmittedEvent{}

		watcher := &eventemitter.Watcher{Concurrent: false, Handle: l.handleRinghashSubmitEvent}
		eventemitter.On(contract.Id.Hex(), watcher)

		l.events[contract.Id] = contract
		log.Debugf("extracotr,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range l.accessor.RinghashRegistryAbi.Methods {
		if name != BATCHSUBMITRINGHASH_METHOD_NAME && name != SUBMITRINGHASH_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, l.accessor.ProtocolImplAbi)
		watcher := &eventemitter.Watcher{}

		switch contract.Name {
		case SUBMITRINGHASH_METHOD_NAME:
			contract.Method = &ethaccessor.SubmitRingHashMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleSubmitRingHashMethod}
		case BATCHSUBMITRINGHASH_METHOD_NAME:
			contract.Method = &ethaccessor.BatchSubmitRingHashMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleBatchSubmitRingHashMethod}
		}

		eventemitter.On(contract.Id, watcher)
		l.methods[contract.Id] = contract
		log.Debugf("extracotr,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (l *ExtractorServiceImpl) loadTokenTransferDelegateProtocol() {
	for name, event := range l.accessor.DelegateAbi.Events {
		if name != ADDRESSAUTHORIZED_EVT_NAME && name != ADDRESSDEAUTHORIZED_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, l.accessor.DelegateAbi)

		switch contract.Name {
		case ADDRESSAUTHORIZED_EVT_NAME:
			contract.Event = &ethaccessor.AddressAuthorizedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleAddressAuthorizedEvent}
		case ADDRESSDEAUTHORIZED_EVT_NAME:
			contract.Event = &ethaccessor.AddressDeAuthorizedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: l.handleAddressDeAuthorizedEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		l.events[contract.Id] = contract
		log.Debugf("extracotr,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}
