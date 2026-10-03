package deliberation

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// execute answers one tool call: an unknown tool or arguments that fail the
// tool's schema are refused without running anything; a tool that does
// not return within PerToolTimeout stops the deliberation; any other tool
// error is reported to the model (the tool itself records what its failure
// means, e.g. an inconclusive scan).
func (s Session) execute(ctx context.Context, round int, tools map[string]Tool, call llm.ToolCall) (llm.Message, Call, error) {
	record := Call{Round: round, ID: call.ID, Tool: call.Name, Arguments: s.redact(string(call.Arguments))}
	tool, ok := tools[call.Name]
	if !ok {
		return s.refuse(record, call, fmt.Errorf("ferramenta %q não foi oferecida", call.Name))
	}
	if err := ValidateArguments(tool.Spec().Parameters, call.Arguments); err != nil {
		return s.refuse(record, call, err)
	}
	runCtx, cancel := context.WithTimeout(ctx, s.Limits.PerToolTimeout)
	defer cancel()
	started := s.now()
	result, err := tool.Run(runCtx, call.Arguments)
	record.DurationMS = s.now().Sub(started).Milliseconds()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		record.Status = StatusFailed
		record.Result = LimitPerToolTimeout
		return llm.Message{}, record, &LimitError{Limit: LimitPerToolTimeout, Detail: fmt.Sprintf("%s excedeu %s", call.Name, s.Limits.PerToolTimeout)}
	}
	if err != nil {
		record.Status = StatusFailed
		record.Result = s.redact(err.Error())
		return toolMessage(call, "falha da ferramenta: "+record.Result), record, nil
	}
	record.Status = StatusExecuted
	record.Result = s.redact(result.Summary)
	record.Digest = result.Digest
	return toolMessage(call, capContent(s.redact(result.Content))), record, nil
}

// refuse records a call that was never executed and tells the model why.
func (s Session) refuse(record Call, call llm.ToolCall, reason error) (llm.Message, Call, error) {
	record.Status = StatusRefused
	record.Result = s.redact(reason.Error())
	return toolMessage(call, "chamada recusada antes de executar: "+record.Result), record, nil
}

func toolMessage(call llm.ToolCall, content string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: content}
}

// capContent keeps a tool message within maxToolMessageBytes.
func capContent(content string) string {
	if len(content) <= maxToolMessageBytes {
		return content
	}
	cut := maxToolMessageBytes - len(truncationNote)
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	return content[:cut] + truncationNote
}
