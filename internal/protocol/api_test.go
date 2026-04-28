package protocol

import (
	"encoding/base64"
	"testing"
)

func TestChatAndResponsesImageParsing(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	body := map[string]any{
		"model": "gpt-image-2",
		"messages": []any{
			map[string]any{"role": "system", "content": "ignore"},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "画一张图"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + imageData}},
			}},
		},
		"n": 2,
	}

	model, prompt, n, images, err := ChatImageArgs(body)
	if err != nil {
		t.Fatalf("ChatImageArgs() error = %v", err)
	}
	if model != "gpt-image-2" || prompt != "画一张图" || n != 2 {
		t.Fatalf("ChatImageArgs() = model %q prompt %q n %d", model, prompt, n)
	}
	if len(images) != 1 || string(images[0].Data) != "png-bytes" || images[0].ContentType != "image/png" {
		t.Fatalf("images = %#v", images)
	}

	responseInput := []any{
		map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": "生成封面"},
			map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + imageData},
		}},
	}
	if prompt := ExtractResponsePrompt(responseInput); prompt != "生成封面" {
		t.Fatalf("ExtractResponsePrompt() = %q", prompt)
	}
	if image := ExtractResponseImage(responseInput); image == nil || string(image.Data) != "png-bytes" {
		t.Fatalf("ExtractResponseImage() = %#v", image)
	}
}

func TestToolCallParsing(t *testing.T) {
	text := `先处理
<tool_calls><tool_call><tool_name>read_file</tool_name><parameters><path><![CDATA[internal/app.go]]></path><limit>5</limit></parameters></tool_call></tool_calls>`
	calls := ParseToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("ParseToolCalls() = %#v", calls)
	}
	if calls[0].Name != "read_file" {
		t.Fatalf("tool name = %q", calls[0].Name)
	}
	if calls[0].Input["path"] != "internal/app.go" || calls[0].Input["limit"] != float64(5) {
		t.Fatalf("tool input = %#v", calls[0].Input)
	}
	if visible := StreamableText(text); visible != "先处理" {
		t.Fatalf("StreamableText() = %q", visible)
	}
	if stripped := StripToolMarkup(text); stripped != "先处理" {
		t.Fatalf("StripToolMarkup() = %q", stripped)
	}
}

func TestStreamImageResponseErrorsWhenNoImageOutput(t *testing.T) {
	outputs := make(chan ImageOutput)
	close(outputs)
	events, errCh := StreamImageResponse(outputs, "draw", "gpt-image-2")
	var count int
	for range events {
		count++
	}
	if count != 1 {
		t.Fatalf("event count = %d, want response.created only", count)
	}
	if err := <-errCh; err == nil || err.Error() != "image generation failed" {
		t.Fatalf("err = %v", err)
	}
}
