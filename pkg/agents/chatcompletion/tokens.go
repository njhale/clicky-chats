package chatcompletion

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gptscript-ai/clicky-chats/pkg/db"
	"github.com/pkoukk/tiktoken-go"
)

// countPromptTokens returns an estimate of the number of prompt tokens that will be generated by an OpenAI model
// for a given chat completion request.
func countPromptTokens(model string, cc *db.CreateChatCompletionRequest) (int, error) {
	if cc == nil {
		return 0, fmt.Errorf("nil request, can't count tokens")
	}

	tkm, err := tiktoken.EncodingForModel(model)
	if err != nil {
		return 0, fmt.Errorf("failed to get encoding for model %s: %w", model, err)
	}

	fixedCost := fixedTokenCost{
		// TODO(njhale): These may differ per model. Do some tests to confirm they're accurate for all the models supported by this function.
		toolParameters:                   11,
		tools:                            12,
		toolParameterPropertyType:        2,
		toolParameterPropertyDescription: 2,
		toolParameterPropertyEnum:        -3,
		toolParameterPropertyEnumElement: 3,
	}
	switch model {
	case "gpt-3.5-turbo-0613",
		"gpt-3.5-turbo-16k-0613",
		"gpt-4-0314",
		"gpt-4-32k-0314",
		"gpt-4-0613",
		"gpt-4-32k-0613":
		fixedCost.message = 3
		fixedCost.name = 1
	case "gpt-3.5-turbo-0301":
		fixedCost.message = 4 // every message follows <|start|>{role/name}\n{content}<|end|>\n
		fixedCost.name = -1   // if there's a name, the role is omitted
	default:
		if strings.Contains(model, "gpt-3.5-turbo") {
			// gpt-3.5-turbo may update over time. Returning num tokens assuming gpt-3.5-turbo-0613.
			return countPromptTokens("gpt-3.5-turbo-0613", cc)
		}
		if strings.Contains(model, "gpt-4") {
			// gpt-4 may update over time. Returning num tokens assuming gpt-4-0613.
			return countPromptTokens("gpt-4-0613", cc)
		}

		return 0, fmt.Errorf("token counting method for model %s is unknown", model)
	}

	req, err := toTokenRequest(cc)
	if err != nil {
		return 0, fmt.Errorf("failed to convert chat completion request to token counting request: %w", err)
	}

	count := func(s string) int {
		return len(tkm.Encode(s, nil, nil))
	}

	// Sum prompt tokens from explicit messages
	var tokens int
	for _, msg := range req.Messages {
		tokens += fixedCost.message
		for _, s := range []string{msg.Content, msg.Role, msg.Name} {
			tokens += count(s)
		}
		if msg.Name != "" {
			tokens += fixedCost.name
		}
	}

	// Sum prompt tokens from function definitions
	// Note: According to https://community.openai.com/t/how-to-calculate-the-tokens-when-using-function-call/266573/6,
	// tool definitions are transformed into system messages with an undocumented encoding scheme before being passed
	// to the LLM. https://community.openai.com/t/how-to-calculate-the-tokens-when-using-function-call/266573/10 suggests
	// a counting implementation based on reverse-engineering token counts for non-streaming requests with tool definitions.
	// TODO(njhale): Try https://community.openai.com/t/how-to-calculate-the-tokens-when-using-function-call/266573/57 instead
	// TODO(njhale): Write some test cases to determine accuracy of this solution
	//for _, tool := range req.Tools {
	//	function := tool.Function
	//	if tool.Function == nil || tool.Type != "function" {
	//		continue
	//	}
	//
	//	for _, s := range []string{function.Description, function.Name} {
	//		tokens += count(s)
	//	}
	//
	//	for _, parameter := range function.Parameters {
	//		for propertyName, property := range parameter.Properties {
	//			tokens += count(propertyName)
	//
	//			if propertyType := property.Type; propertyType != "" {
	//				tokens += fixedCost.toolParameterPropertyType + count(propertyType)
	//			}
	//			if propertyDesc := property.Description; propertyDesc != "" {
	//				tokens += fixedCost.toolParameterPropertyDescription + count(propertyDesc)
	//			}
	//			if propertyEnum := property.Enum; propertyEnum != nil {
	//				tokens += fixedCost.toolParameterPropertyEnum
	//				for _, e := range propertyEnum {
	//					tokens += fixedCost.toolParameterPropertyEnumElement
	//					if s, ok := e.(string); ok {
	//						tokens += count(s)
	//					}
	//				}
	//			}
	//		}
	//	}
	//	if len(function.Parameters) > 0 {
	//		tokens += fixedCost.toolParameters
	//	}
	//}
	//
	//if len(req.Tools) > 0 {
	//	tokens += fixedCost.tools
	//}

	return tokens, nil
}

type fixedTokenCost struct {
	message                          int
	name                             int
	tools                            int
	toolParameters                   int
	toolParameterPropertyType        int
	toolParameterPropertyDescription int
	toolParameterPropertyEnum        int
	toolParameterPropertyEnumElement int
}

type tokenRequest struct {
	Messages []tokenMessage `json:"messages"`
	//Tools    []tokenTool    `json:"tools"`
}

type tokenMessage struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

//type tokenTool struct {
//	Type     string         `json:"type"`
//	Function *tokenFunction `json:"function"`
//}
//
//type tokenFunction struct {
//	Name        string           `json:"name"`
//	Description string           `json:"description"`
//	Parameters  []tokenParameter `json:"parameters"`
//}
//
//type tokenParameter struct {
//	Properties map[string]tokenProperty `json:"properties"`
//}
//
//type tokenProperty struct {
//	Type        string `json:"type"`
//	Description string `json:"description"`
//	Enum        []any  `json:"enum"`
//}

func toTokenRequest(from *db.CreateChatCompletionRequest) (*tokenRequest, error) {
	data, err := json.Marshal(from)
	if err != nil {
		return nil, err
	}

	var to tokenRequest
	if err := json.Unmarshal(data, &to); err != nil {
		return nil, err
	}

	return &to, nil
}
