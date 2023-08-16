package monitor

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	gql "github.com/hasura/go-graphql-client"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/ojo-network/ethIndexerMonitor/config"
)

type (
	BundleQuery struct {
		Bundle struct {
			EthPriceUSD string `graphql:"ethPriceUSD"`
			ID          string `graphql:"id"`
		} `graphql:"bundle(id: \"1\")"`
	}

	ethChecker struct {
		logger       zerolog.Logger
		network      string
		truthUrl     *gql.Client
		checkUrl     *gql.Client
		baseAsset    string
		deviation    float64
		cronDuration time.Duration

		truth float64
		check float64

		mut sync.Mutex
	}

	EthService struct {
		services map[string]*ethChecker
	}
)

func StartEthServices(ctx context.Context, logger zerolog.Logger, config config.Config) *EthService {
	eLogger := logger.With().Str("module", "eth-service").Logger()
	es := EthService{
		services: make(map[string]*ethChecker),
	}

	for network, details := range config.NetworkMap {
		cronDuration, _ := time.ParseDuration(details.CronInterval)
		service := newEthMonitorService(ctx, eLogger, details.Deviation, network, details.TruthUrl, details.CheckUrl, details.BaseAsset, cronDuration)
		es.services[network] = service

		eLogger.Info().Str("network", network).
			Str("base_asset", details.BaseAsset).
			Str("check_url", details.CheckUrl).
			Str("truth_url", details.TruthUrl).
			Msg("monitoring")
	}

	return &es
}

func newEthMonitorService(ctx context.Context, logger zerolog.Logger, deviation float64, network, truthUrl, checkUrl, baseAsset string, cronDuration time.Duration) *ethChecker {
	service := &ethChecker{
		logger:       logger.With().Str("network", network).Logger(),
		deviation:    deviation,
		network:      network,
		cronDuration: cronDuration,
		truthUrl:     gql.NewClient(truthUrl, nil),
		checkUrl:     gql.NewClient(checkUrl, nil),
		baseAsset:    baseAsset,
	}

	go service.startCron(ctx)

	return service
}

func (es *ethChecker) startCron(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			wg.Done()
			return

		default:
			err := es.checkAssetPrice(ctx)
			if err != nil {
				es.logger.Err(err).
					Str("network", es.network).
					Msg("Error in querying request ids")
			}

			time.Sleep(es.cronDuration)
		}
	}
}

func (es *ethChecker) checkAssetPrice(ctx context.Context) error {
	g, _ := errgroup.WithContext(ctx)
	var (
		truth float64
		check float64
	)

	g.Go(func() error {
		price, err := GetBundle(es.truthUrl)
		if err != nil {
			return err
		}

		truth = price

		return nil
	})

	g.Go(func() error {
		price, err := GetBundle(es.checkUrl)
		if err != nil {
			return err
		}

		check = price

		return nil
	})

	err := g.Wait()
	if err != nil {
		return err
	}

	if truth != check {
		// check deviation
		if (math.Abs(truth-check)/truth)*100 > es.deviation {
			slackChan <- createMismatchedDeviationAttachment(
				truth,
				check,
				es.network,
				es.baseAsset,
			)
		}
	}

	es.check = check
	es.truth = truth

	return nil
}

func GetBundle(c *gql.Client) (float64, error) {
	var bundle BundleQuery
	err := c.Query(context.Background(), &bundle, nil)
	if err != nil {
		return 0, err
	}

	return strconv.ParseFloat(bundle.Bundle.EthPriceUSD, 64)
}

func (ec *EthService) getPrices(network string) (float64, float64, error) {
	service, found := ec.services[network]
	if !found {
		return 0, 0, fmt.Errorf("network %s not found", network)
	}

	service.mut.Lock()
	defer service.mut.Unlock()

	return service.check, service.truth, nil
}
