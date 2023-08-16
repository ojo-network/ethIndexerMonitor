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

	network := commands[0]
	switch command.Command {
	case "/netstatus":
		check, truth, err := es.getPrices(network)
		if err != nil {
			return err
		}

		slackChan <- checkPriceAttachment(check, truth, network)

	}
	return nil
}
