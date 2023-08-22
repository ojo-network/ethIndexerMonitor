package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

func NewEventService(ctx context.Context, ethService *EthService, logger zerolog.Logger, slackClient *slack.Client) error {
	client := socketmode.New(
		slackClient,
	)

	log := logger.With().Str("service", "event").Logger()
	wg.Add(1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				wg.Done()
				return

			case evt := <-client.Events:
				switch evt.Type {
				case socketmode.EventTypeConnecting:
					log.Info().Msg("Connecting to Slack with Socket Mode...")
				case socketmode.EventTypeConnectionError:
					log.Err(fmt.Errorf("connection failed. Retrying")).Send()
				case socketmode.EventTypeSlashCommand:
					command, ok := evt.Data.(slack.SlashCommand)
					if !ok {
						continue
					}

					client.Ack(*evt.Request)

					err := handleSlashCommand(ethService, &command)
					if err != nil {
						slackChan <- postErr(err)
					}
				}
			}
		}
	}()

	return client.RunContext(ctx)
}

func handleSlashCommand(es *EthService, command *slack.SlashCommand) error {
	commands := strings.Split(command.Text, " ")
	if len(commands) < 1 {
		return fmt.Errorf("no network")
	}

	switch command.Command {
	case "/netstatus":
		if len(commands) < 2 {
			return fmt.Errorf("no token name")
		}

		network, tokenName := commands[0], commands[1]
		check, truth, err := es.getPrices(network, tokenName)
		if err != nil {
			return err
		}

		slackChan <- checkPriceAttachment(check, truth, tokenName, network)

	case "/listassets":
		names, ids, err := es.getAllTokenNames(commands[0])
		if err != nil {
			return err
		}

		slackChan <- listAttachment(commands[0], names, ids)
	}
	return nil
}
