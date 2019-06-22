package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func (s *IntegrationTestSuite) TestIBCTokenTransfer() {
	var ibcStakeDenom string
	s.Run("send_stake_to_hilo", func() {
		recipient := s.chain.validators[0].keyInfo.GetAddress().String()
		token := sdk.NewInt64Coin("stake", 3300000000) // 3300stake
		s.sendIBC(gaiaChainID, s.chain.id, recipient, token)

		hiloAPIEndpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))

		// require the recipient account receives the IBC tokens (IBC packets ACKd)
		var (
			balances sdk.Coins
			err      error
		)
		s.Require().Eventually(
			func() bool {
				balances, err = queryHiloAllBalances(hiloAPIEndpoint, recipient)
				s.Require().NoError(err)

				return balances.Len() == 3
			},
			time.Minute,
			5*time.Second,
		)

		for _, c := range balances {
			if strings.Contains(c.Denom, "ibc/") {
				ibcStakeDenom = c.Denom
				break
			}
		}

		s.Require().NotEmpty(ibcStakeDenom)
	})

	var ibcStakeERC20Addr string
	s.Run("deploy_stake_erc20", func() {
		ibcStakeERC20Addr = s.deployERC20Token(ibcStakeDenom)
		s.Require().NotEmpty(ibcStakeERC20Addr)

		_, err := hexutil.Decode(ibcStakeERC20Addr)
		s.Require().NoError(err)
	})

	// send 300 stake tokens from Hilo to Ethereum
	s.Run("send_stake_tokens_to_eth", func() {
		ethRecipient := s.chain.validators[1].ethereumKey.address
		s.sendFromHiloToEth(0, ethRecipient, fmt.Sprintf("300%s", ibcStakeDenom), "10photon", fmt.Sprintf("7%s", ibcStakeDenom))

		hiloAPIEndpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))
		fromAddr := s.chain.validators[0].keyInfo.GetAddress()

		// NOTE: We have to query for all balances because currently querying by
		// IBC denomination will result in an API error due to the denomination not
		// being URI handled.
		var token sdk.Coin
		balances, err := queryHiloAllBalances(hiloAPIEndpoint, fromAddr.String())
		s.Require().NoError(err)

		for _, c := range balances {
			if c.Denom == ibcStakeDenom {
				token = c
				break
			}
		}

		s.Require().Equal(ibcStakeDenom, token.Denom)
		s.Require().Equal(int64(3299999693), token.Amount.Int64())

		// require the Ethereum recipient balance increased
		s.Require().Eventually(
			func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				b, err := queryEthTokenBalance(ctx, s.ethClient, ibcStakeERC20Addr, ethRecipient)
				if err != nil {
					return false
				}

				// The balance could differ if the receiving address was the orchestrator
				// the sent the batch tx and got the gravity fee.
				return b >= 300 && b <= 307
			},
			2*time.Minute,
			5*time.Second,
		)
	})

	// send 300 stake tokens from Ethereum back to Hilo
	s.Run("send_stake_tokens_from_eth", func() {
		s.sendFromEthToHilo(1, ibcStakeERC20Addr, s.chain.validators[0].keyInfo.GetAddress().String(), "300")

		hiloAPIEndpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))
		toAddr := s.chain.validators[0].keyInfo.GetAddress()
		expBalance := int64(3299999993)

		// require the original sender's (validator) balance increased
		s.Require().Eventually(
			func() bool {
				// NOTE: We have to query for all balances because currently querying by
				// IBC denomination will result in an API error due to the denomination
				// not being URI handled.
				var token sdk.Coin
				balances, err := queryHiloAllBalances(hiloAPIEndpoint, toAddr.String())
				s.Require().NoError(err)

				for _, c := range balances {
					if c.Denom == ibcStakeDenom {
						token = c
						break
					}
				}

				return token.Amount.Int64() == expBalance
			},
			2*time.Minute,
			5*time.Second,
		)
	})
}

func (s *IntegrationTestSuite) TestPhotonTokenTransfers() {
	// deploy photon ERC20 token contact
	var photonERC20Addr string
	s.Run("deploy_photon_erc20", func() {
		photonERC20Addr = s.deployERC20Token("photon")
		s.Require().NotEmpty(photonERC20Addr)

		_, err := hexutil.Decode(photonERC20Addr)
		s.Require().NoError(err)
	})

	// send 100 photon tokens from Hilo to Ethereum
	s.Run("send_photon_tokens_to_eth", func() {
		ethRecipient := s.chain.validators[1].ethereumKey.address
		s.sendFromHiloToEth(0, ethRecipient, "100photon", "10photon", "3photon")

		hiloEndpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))
		fromAddr := s.chain.validators[0].keyInfo.GetAddress()

		// require the sender's (validator) balance decreased
		balance, err := queryHiloDenomBalance(hiloEndpoint, fromAddr.String(), "photon")
		s.Require().NoError(err)
		s.Require().Equal(int64(99999999084), balance.Amount.Int64())

		// require the Ethereum recipient balance increased
		s.Require().Eventually(
			func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				b, err := queryEthTokenBalance(ctx, s.ethClient, photonERC20Addr, ethRecipient)
				if err != nil {
					return false
				}

				// The balance could differ if the receiving address was the orchestrator
				// the sent the batch tx and got the gravity fee.
				return b >= 100 && b <= 103
			},
			2*time.Minute,
			5*time.Second,
		)
	})

	// send 100 photon tokens from Ethereum back to Hilo
	s.Run("send_photon_tokens_from_eth", func() {
		s.sendFromEthToHilo(1, photonERC20Addr, s.chain.validators[0].keyInfo.GetAddress().String(), "100")

		hiloEndpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))
		toAddr := s.chain.validators[0].keyInfo.GetAddress()
		expBalance := int64(99999999184)

		// require the original sender's (validator) balance increased
		s.Require().Eventually(
			func() bool {
				b, err := queryHiloDenomBalance(hiloEndpoint, toAddr.String(), "photon")
				if err != nil {
					return false
				}

				return b.Amount.Int64() == expBalance
			},
			2*time.Minute,
			5*time.Second,
		)
	})
}

func (s *IntegrationTestSuite) TestHiloTokenTransfers() {
	// deploy hilo ERC20 token contract
	var hiloERC20Addr string
	s.Run("deploy_hilo_erc20", func() {
		hiloERC20Addr = s.deployERC20Token("uhilo")
		s.Require().NotEmpty(hiloERC20Addr)

		_, err := hexutil.Decode(hiloERC20Addr)
		s.Require().NoError(err)
	})

	// send 300 hilo tokens from Hilo to Ethereum
	s.Run("send_uhilo_tokens_to_eth", func() {
		ethRecipient := s.chain.validators[1].ethereumKey.address
		s.sendFromHiloToEth(0, ethRecipient, "300uhilo", "10photon", "7uhilo")

		endpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))
		fromAddr := s.chain.validators[0].keyInfo.GetAddress()

		balance, err := queryHiloDenomBalance(endpoint, fromAddr.String(), "uhilo")
		s.Require().NoError(err)
		s.Require().Equal(int64(9999999693), balance.Amount.Int64())

		// require the Ethereum recipient balance increased
		s.Require().Eventually(
			func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				b, err := queryEthTokenBalance(ctx, s.ethClient, hiloERC20Addr, ethRecipient)
				if err != nil {
					return false
				}

				// The balance could differ if the receiving address was the orchestrator
				// the sent the batch tx and got the gravity fee.
				return b >= 300 && b <= 307
			},
			2*time.Minute,
			5*time.Second,
		)
	})

	// send 300 hilo tokens from Ethereum back to Hilo
	s.Run("send_uhilo_tokens_from_eth", func() {
		s.sendFromEthToHilo(1, hiloERC20Addr, s.chain.validators[0].keyInfo.GetAddress().String(), "300")

		hiloEndpoint := fmt.Sprintf("http://%s", s.valResources[0].GetHostPort("1317/tcp"))
		toAddr := s.chain.validators[0].keyInfo.GetAddress()
		expBalance := int64(9999999993)

		// require the original sender's (validator) balance increased
		s.Require().Eventually(
			func() bool {
				b, err := queryHiloDenomBalance(hiloEndpoint, toAddr.String(), "uhilo")
				if err != nil {
					return false
				}

				return b.Amount.Int64() == expBalance
			},
			2*time.Minute,
			5*time.Second,
		)
	})
}
