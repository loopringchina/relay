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

package market

import (
	"errors"
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/market/util"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/patrickmn/go-cache"
	"math/big"
	"strings"
)

type Account struct {
	Address    string
	Balances   map[string]Balance
	Allowances map[string]Allowance
}

type Balance struct {
	Token   string
	Balance *big.Int
}

type Allowance struct {
	//contractVersion string
	token     string
	allowance *big.Int
}

type AccountManager struct {
	c                 *cache.Cache
	accessor          *ethaccessor.EthNodeAccessor
	newestBlockNumber types.Big
}

type Token struct {
	Token     string `json:"token"`
	Balance   string `json:"balance"`
	Allowance string `json:"allowance"`
}

type AccountJson struct {
	ContractVersion string  `json:"contractVersion"`
	Address         string  `json:"owner"`
	Tokens          []Token `json:"tokens"`
}

func NewAccountManager(accessor *ethaccessor.EthNodeAccessor) AccountManager {

	accountManager := AccountManager{accessor: accessor}
	var blockNumber types.Big
	err := accessor.Call(&blockNumber, "eth_blockNumber")
	if err != nil {
		log.Fatal("init account manager failed, can't get newest block number")
		return accountManager
	}
	accountManager.newestBlockNumber = blockNumber
	accountManager.c = cache.New(cache.NoExpiration, cache.NoExpiration)
	transferWatcher := &eventemitter.Watcher{Concurrent: false, Handle: accountManager.HandleTokenTransfer}
	approveWatcher := &eventemitter.Watcher{Concurrent: false, Handle: accountManager.HandleApprove}
	eventemitter.On(eventemitter.AccountTransfer, transferWatcher)
	eventemitter.On(eventemitter.AccountApproval, approveWatcher)

	return accountManager
}

func (a *AccountManager) GetBalance(contractVersion, address string) Account {

	address = strings.ToLower(address)
	accountInCache, ok := a.c.Get(address)
	if ok {
		account := accountInCache.(Account)
		return account
	} else {
		account := Account{Address: address, Balances: make(map[string]Balance), Allowances: make(map[string]Allowance)}
		for k, v := range util.AllTokens {
			balance := Balance{Token: k}

			amount, err := a.GetBalanceFromAccessor(v.Symbol, address)
			if err != nil {
				log.Infof("get balance failed, token:%s", v.Symbol)
			} else {
				balance.Balance = amount
				account.Balances[k] = balance
			}

			allowance := Allowance{
				//contractVersion: contractVersion,
				token: k}

			allowanceAmount, err := a.GetAllowanceFromAccessor(v.Symbol, address, contractVersion)
			if err != nil {
				log.Infof("get allowance failed, token:%s", v.Symbol)
			} else {
				allowance.allowance = allowanceAmount
				account.Allowances[buildAllowanceKey(contractVersion, k)] = allowance
			}

		}
		a.c.Set(address, account, cache.NoExpiration)
		return account
	}
}

func (a *AccountManager) GetCutoff(contract, address string) (int, error) {
	cutoffTime, err := a.accessor.GetCutoff(common.StringToAddress(contract), common.StringToAddress(address), "latest")
	return int(cutoffTime.Int64()), err
}

func (a *AccountManager) HandleTokenTransfer(input eventemitter.EventData) (err error) {
	event := input.(*types.TransferEvent)
	if event.Blocknumber.Cmp(a.newestBlockNumber.BigInt()) < 0 {
		log.Info("the eth network may be forked. flush all cache")
		a.c.Flush()
		a.newestBlockNumber = *types.NewBigPtr(big.NewInt(-1))
	} else {
		a.updateBalance(*event, true)
		a.updateBalance(*event, false)
	}
	return nil
}

func (a *AccountManager) HandleApprove(input eventemitter.EventData) (err error) {
	event := input.(*types.ApprovalEvent)
	if event.Blocknumber.Cmp(a.newestBlockNumber.BigInt()) < 0 {
		log.Info("the eth network may be forked. flush all cache")
		a.c.Flush()
		a.newestBlockNumber = *types.NewBigPtr(big.NewInt(-1))
	} else {
		err = a.updateAllowance(*event)
		log.Error(err.Error())
	}
	return
}

func (a *AccountManager) GetBalanceFromAccessor(token string, owner string) (*big.Int, error) {
	return a.accessor.Erc20Balance(util.AllTokens[token].Protocol, common.HexToAddress(owner), "latest")
}

func (a *AccountManager) GetAllowanceFromAccessor(token, owner, spender string) (*big.Int, error) {
	spenderAddress, err := a.accessor.GetSenderAddress(common.HexToAddress(util.ContractVersionConfig[spender]))
	if err != nil {
		return big.NewInt(0), errors.New("invalid spender address")
	}
	return a.accessor.Erc20Allowance(util.AllTokens[token].Protocol, common.HexToAddress(owner), spenderAddress, "latest")
}

func buildAllowanceKey(version, token string) string {
	//return version + "_" + token
	return token
}

func (a *AccountManager) updateBalance(event types.TransferEvent, isAdd bool) error {
	tokenAlias := util.AddressToAlias(event.ContractAddress.String())

	var address string
	if !isAdd {
		address = event.From.String()
	} else {
		address = event.To.String()
	}

	if tokenAlias == "" {
		return errors.New("unsupported token type : " + tokenAlias)
	}

	v, ok := a.c.Get(address)
	if ok {
		account := v.(Account)
		balance, ok := account.Balances[tokenAlias]
		if !ok {
			balance = Balance{Token: tokenAlias}
			amount, err := a.GetBalanceFromAccessor(event.ContractAddress.String(), tokenAlias)
			if err != nil {
				log.Error("get balance failed from accessor")
			} else {
				balance.Balance = amount
			}
			account.Balances[tokenAlias] = balance
		} else {
			oldBalance := balance.Balance
			if isAdd {
				balance.Balance = oldBalance.Sub(oldBalance, event.Value)
			} else {
				balance.Balance = oldBalance.Add(oldBalance, event.Value)
			}
			account.Balances[tokenAlias] = balance
		}
		a.c.Set(address, account, cache.NoExpiration)
	}
	return nil
}

func (a *AccountManager) updateAllowance(event types.ApprovalEvent) error {
	tokenAlias := util.AddressToAlias(event.ContractAddress.String())
	spender := event.Spender.String()
	address := event.Owner.String()

	// 这里只能根据loopring的合约获取了
	spenderAddress, err := a.accessor.GetSenderAddress(common.HexToAddress(util.ContractVersionConfig["v1.0"]))
	if err != nil {
		return errors.New("invalid spender address")
	}

	if strings.ToLower(spenderAddress.Hex()) != strings.ToLower(event.Spender.Hex()) {
		return errors.New("unsupported contract address : " + spender)
	}

	v, ok := a.c.Get(address)
	if ok {
		account := v.(Account)
		allowance := Allowance{
			//contractVersion: spender,
			token:     tokenAlias,
			allowance: event.Value}
		account.Allowances[buildAllowanceKey(spender, tokenAlias)] = allowance
		a.c.Set(address, account, cache.NoExpiration)
	}
	return nil
}

func (account *Account) ToJsonObject(contractVersion string) AccountJson {

	var accountJson AccountJson
	accountJson.Address = account.Address
	accountJson.ContractVersion = contractVersion
	accountJson.Tokens = make([]Token, 0)
	for _, v := range account.Balances {
		allowance := account.Allowances[buildAllowanceKey(contractVersion, v.Token)]
		accountJson.Tokens = append(accountJson.Tokens, Token{v.Token, v.Balance.String(), allowance.allowance.String()})
	}
	return accountJson
}
