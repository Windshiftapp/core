package services

import (
	"context"
	"encoding/json"

	"windshift/internal/models"
	"windshift/internal/operatorcontrols"
)

func (s *ChannelService) checkChannelPolicy(kind, raw string) error {
	controls := operatorcontrols.From(s.db)
	if controls.Allowed(operatorcontrols.GenericMail) && controls.Allowed(operatorcontrols.MailTransport) {
		return nil
	}
	if kind == "smtp" {
		if err := controls.Check(operatorcontrols.MailTransport); err != nil {
			return err
		}
		return controls.Check(operatorcontrols.GenericMail)
	}
	if kind != "email" && kind != "imap" {
		return nil
	}
	var config models.ChannelConfig
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return err
		}
	}
	if kind == "imap" || config.IMAPHost != "" {
		return controls.Check(operatorcontrols.GenericMail)
	}
	return nil
}

func (s *ChannelService) CheckTransportMutation(ctx context.Context, id int) error {
	channel, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return s.checkChannelPolicy(channel.Type, channel.Config)
}

func (s *ChannelService) CheckTransportConfig(ctx context.Context, id int, raw string) error {
	channel, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return s.checkChannelPolicy(channel.Type, raw)
}
