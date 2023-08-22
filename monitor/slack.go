package monitor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/slack-go/slack"
)

func createMismatchedDeviationAttachment(truthPrice, checkPrice float64, network, asset string) slack.Attachment {
	attachment := slack.Attachment{
		Pretext: fmt.Sprintf("*Network*: %s", network),
		Title:   fmt.Sprintf(":exclamation: %s", DeviationExceeded),
		Color:   "danger",
		Fields: []slack.AttachmentField{
			{
				Title: "Network",
				Value: fmt.Sprintf("```%s```", network),
				Short: false,
			},
			{
				Title: "Truth Price",
				Value: fmt.Sprintf("```%v```", truthPrice),
				Short: true,
			},
			{
				Title: "Checked Price",
				Value: fmt.Sprintf("```%v```", checkPrice),
				Short: true,
			},
			{
				Title: "Asset",
				Value: fmt.Sprintf("```%s```", asset),
				Short: true,
			},
		},
		Footer: "Monitor Bot",
		Ts:     json.Number(strconv.FormatInt(time.Now().Unix(), 10)),
	}

	return attachment
}

func checkPriceAttachment(check, truth float64, assetName, network string) slack.Attachment {
	attachment := slack.Attachment{
		Pretext: fmt.Sprintf("*Network*: %s", network),
		Color:   "good",
		Fields: []slack.AttachmentField{
			{
				Title: "Network",
				Value: fmt.Sprintf("```%s```", network),
				Short: false,
			},
			{
				Title: "Asset Name",
				Value: fmt.Sprintf("```%s```", assetName),
				Short: false,
			},
			{
				Title: "Check price",
				Value: fmt.Sprintf("```%v```", check),
				Short: false,
			},
			{
				Title: "Truth price",
				Value: fmt.Sprintf("```%v```", truth),
				Short: true,
			},
		},
		Footer: "Monitor Bot",
		Ts:     json.Number(strconv.FormatInt(time.Now().Unix(), 10)),
	}

	return attachment
}

func postErr(err error) slack.Attachment {
	attachment := slack.Attachment{
		Pretext: "An error has occurred:",
		Text:    fmt.Sprintf("event slash command error: %s", err),
		Color:   "danger",
	}

	return attachment
}
