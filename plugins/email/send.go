package email

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string                          { return "send" }
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return emailServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":           map[string]any{"type": "string"},
			"subject":      map[string]any{"type": "string"},
			"body":         map[string]any{"type": "string"},
			"from":         map[string]any{"type": "string"},
			"cc":           map[string]any{"type": "string"},
			"bcc":          map[string]any{"type": "string"},
			"reply_to":     map[string]any{"type": "string"},
			"content_type": map[string]any{"type": "string"},
		},
		"required": []any{"to", "subject", "body"},
	}
}

type sendExecutor struct{}

func newSendExecutor(_ map[string]any) api.NodeExecutor { return &sendExecutor{} }

func (e *sendExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *sendExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getEmailService(services)
	if err != nil {
		return "", nil, err
	}

	to, err := resolveRecipients(nCtx, config, "to")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}
	if len(to) == 0 {
		return "", nil, fmt.Errorf("email.send: missing required field \"to\"")
	}

	subject, err := resolveRequiredString(nCtx, config, "subject")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	body, err := resolveRequiredString(nCtx, config, "body")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	from, _, _ := resolveString(nCtx, config, "from")
	replyTo, _, _ := resolveString(nCtx, config, "reply_to")

	cc, err := resolveRecipients(nCtx, config, "cc")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	bcc, err := resolveRecipients(nCtx, config, "bcc")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	contentType := "html"
	if ct, ok := config["content_type"].(string); ok {
		contentType = ct
	}

	msg := &Message{
		From:        from,
		To:          to,
		CC:          cc,
		BCC:         bcc,
		ReplyTo:     replyTo,
		Subject:     subject,
		Body:        body,
		ContentType: contentType,
	}

	messageID, err := svc.Send(msg)
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	return "success", map[string]any{"message_id": messageID}, nil
}
