package chatgptclient

import (
	"fmt"
	"math"

	"github.com/go-zoox/core-utils/strings"
	"github.com/go-zoox/lru"

	openai "github.com/go-zoox/openai-client"
)

// Client is the ChatGPT Client.
type Client interface {
	Ask(cfg *AskConfig) ([]byte, error)
	//
	GetOrCreateConversation(id string, cfg *ConversationConfig) (Conversation, error)
	//
	ResetConversations() error
	ResetConversation(id string) error

	//
	ChangeConversationModel(conversationID string, model string) error
}

type client struct {
	core openai.Client
	cfg  *Config
	//
	conversationsCache *lru.LRU
}

// Config is the configuration for the ChatGPT Client.
type Config struct {
	APIKey    string `json:"api_key"`
	APIServer string `json:"api_server"`
	// MaxRequestResponseTokens int    `json:"max_request_response_tokens"`
	MaxResponseTokens    int    `json:"max_response_tokens"`
	MaxConversations     int    `json:"max_conversations"`
	ConversationMaxAge   int    `json:"conversation_max_age"`
	ConversationContext  string `json:"conversation_context"`
	ConversationLanguage string `json:"conversation_language"`
	ChatGPTName          string `json:"chatgpt_name"`

	// Proxy sets the request proxy.
	//
	//	support http, https, socks5
	//	example:
	//		http://127.0.0.1:17890
	//		https://127.0.0.1:17890
	//		socks5://127.0.0.1:17890
	Proxy string `json:"proxy"`
}

// AskConfig ...
type AskConfig struct {
	Model    string     `json:"model"`
	Prompt   string     `json:"prompt"`
	Messages []*Message `json:"messages"`
	//
	MaxRequestResponseTokens int `json:"max_request_response_tokens"`
}

// New creates a new ChatGPT Client.
func New(cfg *Config) (Client, error) {
	// if cfg.MaxRequestResponseTokens == 0 {
	// 	cfg.MaxRequestResponseTokens = DefaultMaxRequestResponseTokens
	// }

	if cfg.MaxResponseTokens == 0 {
		cfg.MaxResponseTokens = DefaultMaxResponseTokens
	}

	if cfg.MaxConversations == 0 {
		cfg.MaxConversations = DefaultMaxConversations
	}

	if cfg.ChatGPTName == "" {
		cfg.ChatGPTName = "ChatGPT"
	}

	core, err := openai.New(&openai.Config{
		APIKey:    cfg.APIKey,
		APIServer: cfg.APIServer,
		Proxy:     cfg.Proxy,
	})
	if err != nil {
		return nil, err
	}

	return &client{
		core:               core,
		cfg:                cfg,
		conversationsCache: lru.New(cfg.MaxConversations),
	}, nil
}

func (c *client) Ask(cfg *AskConfig) (answer []byte, err error) {
	// numTokens := float64(len(question))
	// maxTokens := math.Max(float64(c.cfg.MaxResponseTokens), math.Min(openai.MaxTokens-numTokens, float64(c.cfg.MaxResponseTokens)))

	switch cfg.Model {
	case openai.ModelGPT3_5Turbo, openai.ModelGPT3_5Turbo0301:
		// chat
		currentMessageLength := 0
		messages := []openai.CreateChatCompletionMessage{}
		for _, msg := range cfg.Messages {
			currentMessageLength += len(msg.Text)
			messages = append(messages, openai.CreateChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Text,
			})
		}

		maxTokens := calculationPromptMaxTokens(currentMessageLength, cfg.MaxRequestResponseTokens, c.cfg.MaxResponseTokens)
		completion, err := c.core.CreateChatCompletion(&openai.CreateChatCompletionRequest{
			Model:     cfg.Model,
			Messages:  messages,
			MaxTokens: maxTokens,
			Temperature: 0.1,
		})
		if err != nil {
			return nil, err
		}

		return []byte(strings.TrimSpace(completion.Choices[0].Message.Content)), nil
	}

	// prompt
	questionX := cfg.Prompt
	maxTokens := calculationPromptMaxTokens(len(questionX), cfg.MaxRequestResponseTokens, c.cfg.MaxResponseTokens)

	completion, err := c.core.CreateCompletion(&openai.CreateCompletionRequest{
		Model:     cfg.Model,
		Prompt:    questionX,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return nil, err
	}

	return []byte(strings.TrimSpace(completion.Choices[0].Text)), nil
}

func (c *client) GetOrCreateConversation(id string, cfg *ConversationConfig) (conversation Conversation, err error) {
	if cfg.ID == "" {
		cfg.ID = id
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = DefaultConversationMaxAge
	}
	if cfg.Context == "" && c.cfg.ConversationContext != "" {
		cfg.Context = c.cfg.ConversationContext
	}
	if cfg.Language == "" && c.cfg.ConversationLanguage != "" {
		cfg.Language = c.cfg.ConversationLanguage
	}
	if cfg.ChatGPTName == "" {
		cfg.ChatGPTName = c.cfg.ChatGPTName
	}

	if cache, ok := c.conversationsCache.Get(cfg.ID); ok {
		if c, ok := cache.(Conversation); ok {
			conversation = c
			return conversation, nil
		}
	}

	conversation, err = NewConversation(c, cfg)
	if err != nil {
		return nil, err
	}

	c.conversationsCache.Set(id, conversation, cfg.MaxAge)

	return conversation, nil
}

func (c *client) ResetConversations() error {
	c.conversationsCache.Clear()

	return nil
}

func (c *client) ResetConversation(id string) error {
	c.conversationsCache.Delete(id)

	return nil
}

func (c *client) GetConversation(id string) (conversation Conversation, err error) {
	if cache, ok := c.conversationsCache.Get(id); ok {
		if c, ok := cache.(Conversation); ok {
			return c, nil
		}
	}

	return nil, fmt.Errorf("conversation(id: %s) not found", id)
}

func (c *client) ChangeConversationModel(conversationID string, model string) error {
	conversation, err := c.GetConversation(conversationID)
	if err != nil {
		return err
	}

	return conversation.SetModel(model)
}

func calculationPromptMaxTokens(questLength, MaxRequestResponseTokens, MaxResponseTokens int) int {
	numTokens := questLength
	maxTokens := math.Max(float64(MaxResponseTokens), math.Min(float64(MaxRequestResponseTokens-numTokens), float64(MaxResponseTokens)))

	return int(maxTokens)
}
