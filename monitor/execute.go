package monitor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/slack-go/slack"
	"github.com/spf13/cobra"

	"github.com/ojo-network/ethIndexerMonitor/config"
)

var (
	slackChan chan slack.Attachment
	wg        sync.WaitGroup
)

const (
	DeviationExceeded = "Price diff exceed deviation"
)

var rootCmd = &cobra.Command{
	Use:   "monitor [config-file]",
	Args:  cobra.ExactArgs(1),
	Short: "eth service monitor",
	RunE:  ethCmdHandler,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	rootCmd.AddCommand(getVersionCmd())
}

func ethCmdHandler(cmd *cobra.Command, args []string) error {
	cfg, accessToken, err := config.ParseConfig(args)
	if err != nil {
		return err
	}

	client := slack.New(accessToken.SlackToken, slack.OptionDebug(false), slack.OptionAppLevelToken(accessToken.AppToken))
	slackChan = make(chan slack.Attachment, len(cfg.NetworkMap))

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.InfoLevel).With().Timestamp().Logger()
	if err != nil {
		return err
	}

	logger.Info().Msg("Eth indexer monitor starting...")
	go func() {
		for attachment := range slackChan {
			_, timestamp, err := client.PostMessage(accessToken.SlackChannel, slack.MsgOptionAttachments(attachment))
			if err != nil {
				logger.Err(err).Msg("error posting slack message")
			}

			logger.Info().Str("Posted at timestamp", timestamp).Msg("slack message posted")
		}
	}()

	ctx, cancel := context.WithCancel(cmd.Context())
	// starting cron services
	ets := StartEthServices(ctx, logger, *cfg)

	// starting slash command service
	err = NewEventService(ctx, ets, logger, client)
	if err != nil {
		cancel()
		return err
	}

	trapSignal(cancel)

	<-ctx.Done()
	logger.Info().Msg("closing monitor, waiting for all monitors to exit")
	wg.Wait()

	close(slackChan)
	return nil
}

func trapSignal(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	go func() {
		<-sigCh
		cancel()
	}()
}
