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

package ethaccessor_test

import (
	"github.com/Loopring/relay/crypto"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/test"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"testing"
)

const (
	version              = "v_0_1"
	cancelOrderHash      = "0x50abf49842feb1cb5e145e2835612a2a32534759c7e17484583f0d26b504ac75"
	cutOffOwner          = "0xb1018949b241D76A1AB2094f473E9bEfeAbB5Ead"
	registerTokenAddress = "0x8b62ff4ddc9baeb73d0a3ea49d43e4fe8492935a"
	account1             = "0x1b978a1d302335a6f2ebe4b8823b5e17c3c84135"
	registerTokenSymbol  = "wrdn"
	balanceTokenAddress  = "0x478d07f3cBE07f01B5c7D66b4Ba57e5a3c520564"
	balanceOwner         = ""
	wethAddress          = "0x88699e7fee2da0462981a08a15a3b940304cc516"
	wethOwner            = "0x1b978a1d302335a6f2ebe4b8823b5e17c3c84135"
)

func TestEthNodeAccessor_Erc20Balance(t *testing.T) {
	accessor, err := test.GenerateAccessor()
	if err != nil {
		t.Fatalf("generate accessor error:%s", err.Error())
	}

	tokenAddress := common.HexToAddress(balanceTokenAddress)
	owner := common.HexToAddress(account1)
	balance, err := accessor.Erc20Balance(tokenAddress, owner, "latest")
	if err != nil {
		t.Fatalf("accessor get erc20 balance error:%s", err.Error())
	}

	t.Log(balance.String())
}

func TestEthNodeAccessor_Approval(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(wethOwner)}
	ks.Unlock(account, "201")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call register token
	protocol := common.HexToAddress(wethAddress)
	spender := common.HexToAddress(wethAddress)
	amount, _ := new(big.Int).SetString("1000000000000000000", 0)
	accessor, _ := test.GenerateAccessor()
	callMethod := accessor.ContractSendTransactionMethod(accessor.Erc20Abi, protocol)
	if result, err := callMethod(account, "approve", nil, nil, nil, spender, amount); nil != err {
		t.Fatalf("call method approve error:%s", err.Error())
	} else {
		t.Logf("approve result:%s", result)
	}
}

func TestEthNodeAccessor_Allowance(t *testing.T) {
	accessor, err := test.GenerateAccessor()
	if err != nil {
		t.Fatalf("generate accessor error:%s", err.Error())
	}

	tokenAddress := common.HexToAddress(wethAddress)
	owner := common.HexToAddress(wethOwner)
	spender := common.HexToAddress(wethAddress)

	if allowance, err := accessor.Erc20Allowance(tokenAddress, owner, spender, "latest"); err != nil {
		t.Fatalf("accessor get erc20 approval error:%s", err.Error())
	} else {
		t.Log(allowance.String())
	}
}

func TestEthNodeAccessor_CancelOrder(t *testing.T) {
	var (
		model        *dao.Order
		state        types.OrderState
		err          error
		result       string
		orderhash    = common.HexToHash(cancelOrderHash)
		cancelAmount = big.NewInt(1980)
	)

	// load config
	c := test.Cfg()

	// get order
	rds := test.Rds()
	if model, err = rds.GetOrderByHash(orderhash); err != nil {
		t.Fatalf(err.Error())
	}
	if err := model.ConvertUp(&state); err != nil {
		t.Fatalf(err.Error())
	}

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: state.RawOrder.Owner}
	ks.Unlock(account, "202")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// create cancel order contract function parameters
	addresses := [3]common.Address{state.RawOrder.Owner, state.RawOrder.TokenS, state.RawOrder.TokenB}
	values := [7]*big.Int{state.RawOrder.AmountS, state.RawOrder.AmountB, state.RawOrder.Timestamp, state.RawOrder.Ttl, state.RawOrder.Salt, state.RawOrder.LrcFee, cancelAmount}
	buyNoMoreThanB := state.RawOrder.BuyNoMoreThanAmountB
	marginSplitPercentage := state.RawOrder.MarginSplitPercentage
	v := state.RawOrder.V
	s := state.RawOrder.S
	r := state.RawOrder.R

	// call cancel order
	accessor, _ := test.GenerateAccessor()
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractSendTransactionMethod(accessor.ProtocolImplAbi, protocol)
	if result, err = callMethod(account, "cancelOrder", nil, nil, nil, addresses, values, buyNoMoreThanB, marginSplitPercentage, v, r, s); nil != err {
		t.Fatalf("call method cancelOrder error:%s", err.Error())
	} else {
		t.Logf("cancelOrder result:%s", result)
	}
}

func TestEthNodeAccessor_GetCancelledOrFilled(t *testing.T) {
	c := test.Cfg()
	accessor, _ := test.GenerateAccessor()

	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	if amount, err := accessor.GetCancelledOrFilled(protocol, common.HexToHash(cancelOrderHash), "latest"); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("cancelOrFilled amount:%s", amount.String())
	}
}

// cutoff的值必须在两个块的timestamp之间
func TestEthNodeAccessor_Cutoff(t *testing.T) {
	cutoff := big.NewInt(1522651087)

	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(cutOffOwner)}
	ks.Unlock(account, "202")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call cutoff
	accessor, _ := test.GenerateAccessor()
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractSendTransactionMethod(accessor.ProtocolImplAbi, protocol)
	if result, err := callMethod(account, "setCutoff", nil, nil, nil, cutoff); nil != err {
		t.Fatalf("call method setCutoff error:%s", err.Error())
	} else {
		t.Logf("cutoff result:%s", result)
	}
}

func TestEthNodeAccessor_GetCutoff(t *testing.T) {
	c := test.Cfg()
	accessor, _ := test.GenerateAccessor()

	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	if timestamp, err := accessor.GetCutoff(protocol, common.HexToAddress(cutOffOwner), "latest"); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("cutoff timestamp:%s", timestamp.String())
	}
}

func TestEthNodeAccessor_TokenRegister(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(c.Miner.Miner)}
	ks.Unlock(account, "101")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call register token
	accessor, _ := test.GenerateAccessor()
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractSendTransactionMethod(accessor.TokenRegistryAbi, accessor.ProtocolAddresses[protocol].TokenRegistryAddress)
	if result, err := callMethod(account, "registerToken", nil, nil, nil, common.HexToAddress(registerTokenAddress), registerTokenSymbol); nil != err {
		t.Fatalf("call method registerToken error:%s", err.Error())
	} else {
		t.Logf("registerToken result:%s", result)
	}
}

func TestEthNodeAccessor_TokenUnRegister(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(c.Miner.Miner)}
	ks.Unlock(account, "101")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call unregister token
	accessor, _ := test.GenerateAccessor()
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractSendTransactionMethod(accessor.TokenRegistryAbi, accessor.ProtocolAddresses[protocol].TokenRegistryAddress)
	if result, err := callMethod(account, "unregisterToken", nil, nil, nil, common.HexToAddress(registerTokenAddress), registerTokenSymbol); nil != err {
		t.Fatalf("call method unregisterToken error:%s", err.Error())
	} else {
		t.Logf("unregisterToken result:%s", result)
	}
}

func TestEthNodeAccessor_GetAddressBySymbol(t *testing.T) {
	c := test.Cfg()
	accessor, _ := test.GenerateAccessor()

	var result string
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractCallMethod(accessor.TokenRegistryAbi, accessor.ProtocolAddresses[protocol].TokenRegistryAddress)
	if err := callMethod(&result, "getAddressBySymbol", "latest", registerTokenSymbol); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("symbol map:%s->%s", registerTokenSymbol, common.HexToAddress(result).Hex())
	}
}

// 注册合约
func TestEthNodeAccessor_AuthorizedAddress(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(c.Miner.Miner)}
	ks.Unlock(account, "101")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call authorized protocol address
	accessor, _ := test.GenerateAccessor()
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractSendTransactionMethod(accessor.DelegateAbi, accessor.ProtocolAddresses[protocol].DelegateAddress)
	if result, err := callMethod(account, "authorizeAddress", nil, nil, nil, protocol); nil != err {
		t.Fatalf("call method authorizeAddress error:%s", err.Error())
	} else {
		t.Logf("authorizeAddress result:%s", result)
	}
}

func TestEthNodeAccessor_DeAuthorizedAddress(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(c.Miner.Miner)}
	ks.Unlock(account, "101")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call deAuthorized protocol address
	accessor, _ := test.GenerateAccessor()
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractSendTransactionMethod(accessor.DelegateAbi, accessor.ProtocolAddresses[protocol].DelegateAddress)
	if result, err := callMethod(account, "deauthorizeAddress", nil, nil, nil, protocol); nil != err {
		t.Fatalf("call method deauthorizeAddress error:%s", err.Error())
	} else {
		t.Logf("deauthorizeAddress result:%s", result)
	}
}

func TestEthNodeAccessor_IsAddressAuthorized(t *testing.T) {
	c := test.Cfg()
	accessor, _ := test.GenerateAccessor()

	var result string
	protocol := common.HexToAddress(c.Common.ProtocolImpl.Address[version])
	callMethod := accessor.ContractCallMethod(accessor.DelegateAbi, accessor.ProtocolAddresses[protocol].DelegateAddress)
	if err := callMethod(&result, "isAddressAuthorized", "latest", protocol); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("symbol map:%s->%s", registerTokenSymbol, result)
	}
}

func TestEthNodeAccessor_WethDeposit(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(wethOwner)}
	ks.Unlock(account, "201")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call deAuthorized protocol address
	wethAddr := common.HexToAddress(wethAddress)
	amount, _ := new(big.Int).SetString("100", 0)
	accessor, _ := test.GenerateAccessor()
	callMethod := accessor.ContractSendTransactionMethod(accessor.WethAbi, wethAddr)
	if result, err := callMethod(account, "deposit", nil, nil, amount); nil != err {
		t.Fatalf("call method weth-deposit error:%s", err.Error())
	} else {
		t.Logf("weth-deposit result:%s", result)
	}
}

func TestEthNodeAccessor_WethWithdrawal(t *testing.T) {
	// load config
	c := test.Cfg()

	// unlock account
	ks := keystore.NewKeyStore(c.Keystore.Keydir, keystore.StandardScryptN, keystore.StandardScryptP)
	account := accounts.Account{Address: common.HexToAddress(wethOwner)}
	ks.Unlock(account, "201")
	cyp := crypto.NewCrypto(true, ks)
	crypto.Initialize(cyp)

	// call deAuthorized protocol address
	wethAddr := common.HexToAddress(wethAddress)
	amount, _ := new(big.Int).SetString("100", 0)
	accessor, _ := test.GenerateAccessor()
	callMethod := accessor.ContractSendTransactionMethod(accessor.WethAbi, wethAddr)
	if result, err := callMethod(account, "withdraw", nil, nil, nil, amount); nil != err {
		t.Fatalf("call method weth-withdraw error:%s", err.Error())
	} else {
		t.Logf("weth-withdraw result:%s", result)
	}
}
