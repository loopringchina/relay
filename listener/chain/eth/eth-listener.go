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

package eth

import (
	"errors"
	"github.com/Loopring/ringminer/chainclient"
	"github.com/Loopring/ringminer/chainclient/eth"
	"github.com/Loopring/ringminer/config"
	"github.com/Loopring/ringminer/db"
	"github.com/Loopring/ringminer/log"
	"github.com/Loopring/ringminer/miner"
	"github.com/Loopring/ringminer/orderbook"
	"github.com/Loopring/ringminer/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"sync"
)

/**
区块链的listener, 得到order以及ring的事件，
*/

const (
	BLOCK_HASH_TABLE_NAME       = "block_hash_table"
	TRANSACTION_HASH_TABLE_NAME = "transaction_hash_table"
)

type Whisper struct {
	ChainOrderChan chan *types.OrderState
}

// TODO(fukun):不同的channel，应当交给orderbook统一进行后续处理，可以将channel作为函数返回值、全局变量、参数等方式
type EthClientListener struct {
	options        config.ChainClientOptions
	commOpts       config.CommonOptions
	ethClient      *eth.EthClient
	ob             *orderbook.OrderBook
	db             db.Database
	blockhashTable db.Database
	txhashTable    db.Database
	whisper        *Whisper
	stop           chan struct{}
	lock           sync.RWMutex
}

func NewListener(options config.ChainClientOptions,
	commonOpts config.CommonOptions,
	whisper *Whisper,
	ethClient *eth.EthClient,
	ob *orderbook.OrderBook,
	database db.Database) *EthClientListener {
	var l EthClientListener

	l.options = options
	l.commOpts = commonOpts
	l.whisper = whisper
	l.ethClient = ethClient
	l.ob = ob
	l.db = database
	l.blockhashTable = db.NewTable(l.db, BLOCK_HASH_TABLE_NAME)
	l.txhashTable = db.NewTable(l.db, TRANSACTION_HASH_TABLE_NAME)

	return &l
}

// TODO(fukun): 这里调试调不通,应当返回channel
func (l *EthClientListener) Start() {
	l.stop = make(chan struct{})

	start := l.commOpts.DefaultBlockNumber
	end := l.commOpts.EndBlockNumber

	for {
		// get block data
		inter, err := l.ethClient.BlockIterator(start, end).Next()
		if err != nil {
			log.Errorf("get block hash error:%s", err.Error())
			continue
		}

		// save block index
		block := inter.(*eth.BlockWithTxObject)
		if len(block.Transactions) < 1 {
			log.Errorf("block transaction empty error")
		}
		if err := l.saveBlock(block); err != nil {
			log.Errorf("get block hash error:%s", err.Error())
			continue
		}

		// get transactions with blockhash
		txs := []types.Hash{}
		for _, tx := range block.Transactions {

			// 判断合约地址是否合法
			if !l.judgeContractAddress(tx.To) {
				continue
			}

			// 解析method，获得ring内等orders并发送到orderbook保存
			l.doMethod(tx.Input)

			// 解析event,并发送到orderbook
			var receipt eth.TransactionReceipt
			err := l.ethClient.GetTransactionReceipt(&receipt, tx)
			if err != nil {
				log.Errorf("eth listener get transaction receipt error:%s", err.Error())
				continue
			}
			for _, v := range receipt.Logs {
				if err := l.doEvent(v); err != nil {
					log.Errorf("eth listener do event error:%s", err.Error())
				}
			}

			txhash := types.HexToHash(tx.Hash)
			txs = append(txs, txhash)
		}

		if err := l.saveTransactions(block.Hash, txs); err != nil {
			log.Errorf("eth listener save transactions error:%s", err.Error())
			continue
		}

	}

}

func (l *EthClientListener) Stop() {
	l.lock.Lock()
	defer l.lock.Unlock()

	close(l.stop)
}

// 重启(分叉)时先关停subscribeEvents，然后关
func (l *EthClientListener) Restart() {

}

func (l *EthClientListener) Name() string {
	return "eth-listener"
}

// 解析方法中orders，并发送到orderbook
// 这些orders，不一定来自ipfs
func (l *EthClientListener) doMethod(input string) {
	// todo: unpack event
	// input := tx.Input
	// l.ethClient
}

func (l *EthClientListener) doEvent(v eth.Log) error {
	address := types.HexToAddress(v.Address)
	impl, ok := miner.LoopringInstance.LoopringImpls[address]
	if !ok {
		return errors.New("contract address do not exsit")
	}

	topic := v.Topics[0]
	data := hexutil.MustDecode(v.Data)

	switch topic {
	case impl.OrderFilled.Id():
		evt := chainclient.OrderFilledEvent{}
		if err := impl.OrderFilled.Unpack(&evt, data, v.Topics); err != nil {
			return err
		}

		hash := types.BytesToHash(evt.OrderHash)
		ord, err := l.ob.GetOrder(hash)
		if err != nil {
			return err
		}

		evt.ConvertDown(ord)
		l.whisper.ChainOrderChan <- ord

	case impl.OrderCancelled.Id():
		evt := chainclient.OrderCancelledEvent{}
		if err := impl.OrderCancelled.Unpack(&evt, data, v.Topics); err != nil {
			return err
		}

		hash := types.BytesToHash(evt.OrderHash)
		ord, err := l.ob.GetOrder(hash)
		if err != nil {
			return err
		}

		evt.ConvertDown(ord)
		l.whisper.ChainOrderChan <- ord

	case impl.CutoffTimestampChanged.Id():

	}

	return nil
}

func (l *EthClientListener) judgeContractAddress(addr string) bool {
	for _, v := range l.commOpts.LoopringImpAddresses {
		if addr == v {
			return true
		}
	}
	return false
}
