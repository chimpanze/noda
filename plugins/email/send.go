package email

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string                           { return "send" }
func (d *sendDescriptor) Description() string                    { return "Sends an email via SMTP" }
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return emailServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":           map[string]any{"type": "string", "description": "Recipient email address(es)"},
			"subject":      map[string]any{"type": "string", "description": "Email subject line"},
			"body":         map[string]any{"type": "string", "description": "Email body content"},
			"from":         map[string]any{"type": "string", "description": "Sender address (overrides service default)"},
			"cc":           map[string]any{"type": "string", "description": "CC recipients"},
			"bcc":          map[string]any{"type": "string", "description": "BCC recipients"},
			"reply_to":     map[string]any{"type": "string", "description": "Reply-To address"},
			"content_type": map[string]any{"type": "string", "description": "Content type: html or text"},
		},
		"required": []any{"to", "subject", "body"},
	}
}
func (d *sendDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "null (email was sent)",
		"error":   "SMTP error",
	}
}

type sendExecutor struct{}

func newSendExecutor(_ map[string]any) api.NodeExecutor { return &sendExecutor{} }

func (e *sendExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *sendExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "mailer")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	to, err := resolveRecipients(nCtx, config, "to")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}
	if len(to) == 0 {
		return "", nil, fmt.Errorf("email.send: missing required field \"to\"")
	}

	subject, err := plugin.ResolveString(nCtx, config, "subject")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	body, err := plugin.ResolveString(nCtx, config, "body")
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	from, _, _ := plugin.ResolveOptionalString(nCtx, config, "from")
	replyTo, _, _ := plugin.ResolveOptionalString(nCtx, config, "reply_to")

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

	messageID, err := svc.Send(ctx, msg)
	if err != nil {
		return "", nil, fmt.Errorf("email.send: %w", err)
	}

	return api.OutputSuccess, map[string]any{"message_id": messageID}, nil
}
