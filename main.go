package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/dshills/second-opinion/config"
	"github.com/dshills/second-opinion/llm"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type providerCacheKey struct {
	Provider        string
	ModelOverride   string
	ReasoningEffort string
}

var (
	cfg                   *config.Config
	llmProviders          = make(map[providerCacheKey]llm.Provider)
	optimizedLLMProviders = make(map[providerCacheKey]llm.OptimizedProvider)
	llmProvidersMux       sync.RWMutex
)

func main() {
	// Load configuration
	var err error
	cfg, err = config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("%+v", cfg)
	log.Printf("Loaded configuration from %s", cfg.ConfigType)
	if cfg.OpenAI.APIKey != "" {
		log.Println("OpenAI Enabled")
	}
	if cfg.Google.APIKey != "" {
		log.Println("Google Enabled")
	}
	if cfg.Ollama.Endpoint != "" {
		log.Println("Ollama Enabled")
	}
	if cfg.Mistral.APIKey != "" {
		log.Println("Mistral Enabled")
	}
	if cfg.Anthropic.APIKey != "" {
		log.Println("Anthropic Enabled")
	}
	log.Printf("Default provider: %s", cfg.DefaultProvider)

	// Initialize default LLM provider
	apiKey, model, endpoint := cfg.GetProviderConfig(cfg.DefaultProvider)
	defaultConfig := llm.Config{
		Provider:    cfg.DefaultProvider,
		APIKey:      apiKey,
		Model:       model,
		Endpoint:    endpoint,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	}

	defaultProvider, err := llm.NewProvider(defaultConfig)
	if err != nil {
		log.Fatalf("Failed to initialize default LLM provider: %v", err)
	}

	llmProvidersMux.Lock()
	defaultKey := providerCacheKey{cfg.DefaultProvider, "", ""}
	llmProviders[defaultKey] = defaultProvider
	optimizedLLMProviders[defaultKey] = llm.NewOptimizedProvider(defaultProvider, cfg)
	llmProvidersMux.Unlock()

	s := server.NewMCPServer(
		cfg.ServerName,
		cfg.ServerVersion,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Git diff analysis tool
	gitDiffTool := mcp.NewTool("analyze_git_diff",
		mcp.WithDescription("Analyze git diff output to understand code changes using LLM"),
		mcp.WithString("diff_content",
			mcp.Required(),
			mcp.Description("Git diff output to analyze"),
		),
		mcp.WithBoolean("summarize",
			mcp.Description("Whether to provide a summary of changes"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider to use (openai, google, ollama, mistral)"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (overrides default for provider)"),
		),
		mcp.WithString("reasoning_effort",
			mcp.Description("OpenAI reasoning effort level (default: from config)"),
			mcp.Enum("minimal", "low", "medium", "high"),
		),
	)
	s.AddTool(gitDiffTool, handleGitDiff)

	// Code review tool
	codeReviewTool := mcp.NewTool("review_code",
		mcp.WithDescription("Review code for quality, security, and best practices using LLM"),
		mcp.WithString("code",
			mcp.Required(),
			mcp.Description("Code to review"),
		),
		mcp.WithString("language",
			mcp.Description("Programming language of the code"),
		),
		mcp.WithString("focus",
			mcp.Description("Specific focus area for review (security, performance, style, etc.)"),
			mcp.Enum("security", "performance", "style", "all"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider to use (openai, google, ollama, mistral)"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (overrides default for provider)"),
		),
		mcp.WithString("reasoning_effort",
			mcp.Description("OpenAI reasoning effort level (default: from config)"),
			mcp.Enum("minimal", "low", "medium", "high"),
		),
	)
	s.AddTool(codeReviewTool, handleCodeReview)

	// Commit analysis tool
	commitAnalysisTool := mcp.NewTool("analyze_commit",
		mcp.WithDescription("Analyze a git commit for quality and adherence to best practices using LLM"),
		mcp.WithString("commit_sha",
			mcp.Description("Git commit SHA to analyze (default: HEAD)"),
		),
		mcp.WithString("repo_path",
			mcp.Description("Path to the git repository (default: current directory)"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider to use (openai, google, ollama, mistral)"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (overrides default for provider)"),
		),
		mcp.WithString("reasoning_effort",
			mcp.Description("OpenAI reasoning effort level (default: from config)"),
			mcp.Enum("minimal", "low", "medium", "high"),
		),
	)
	s.AddTool(commitAnalysisTool, handleCommitAnalysis)

	// Get repository info tool
	repoInfoTool := mcp.NewTool("get_repo_info",
		mcp.WithDescription("Get information about a git repository"),
		mcp.WithString("repo_path",
			mcp.Description("Path to the git repository (default: current directory)"),
		),
	)
	s.AddTool(repoInfoTool, handleRepoInfo)

	// Analyze uncommitted work tool
	uncommittedWorkTool := mcp.NewTool("analyze_uncommitted_work",
		mcp.WithDescription("Analyze uncommitted changes in a git repository using LLM"),
		mcp.WithString("repo_path",
			mcp.Description("Path to the git repository (default: current directory)"),
		),
		mcp.WithBoolean("staged_only",
			mcp.Description("Analyze only staged changes (default: false, analyzes all uncommitted changes)"),
		),
		mcp.WithString("provider",
			mcp.Description("LLM provider to use (openai, google, ollama, mistral)"),
		),
		mcp.WithString("model",
			mcp.Description("Model to use (overrides default for provider)"),
		),
		mcp.WithString("reasoning_effort",
			mcp.Description("OpenAI reasoning effort level (default: from config)"),
			mcp.Enum("minimal", "low", "medium", "high"),
		),
	)
	s.AddTool(uncommittedWorkTool, handleAnalyzeUncommittedWork)

	// Start the stdio server
	log.Printf("Starting %s with default provider: %s", cfg.ServerName, cfg.DefaultProvider)
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// getOrCreateProvider gets an existing provider or creates a new one with the specified config
func getOrCreateProvider(providerName, modelOverride, reasoningEffort string) (llm.Provider, error) {
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}

	re := reasoningEffort
	if re == "" {
		re = cfg.OpenAI.ReasoningEffort
	}

	cacheKey := providerCacheKey{providerName, modelOverride, re}

	llmProvidersMux.RLock()
	if provider, exists := llmProviders[cacheKey]; exists {
		llmProvidersMux.RUnlock()
		return provider, nil
	}
	llmProvidersMux.RUnlock()

	apiKey, model, endpoint := cfg.GetProviderConfig(providerName)
	if modelOverride != "" {
		model = modelOverride
	}

	providerConfig := llm.Config{
		Provider:        providerName,
		APIKey:          apiKey,
		Model:           model,
		Endpoint:        endpoint,
		Temperature:     cfg.Temperature,
		MaxTokens:       cfg.MaxTokens,
		ReasoningEffort: re,
	}

	provider, err := llm.NewProvider(providerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s provider: %w", providerName, err)
	}

	llmProvidersMux.Lock()
	llmProviders[cacheKey] = provider
	optimizedLLMProviders[cacheKey] = llm.NewOptimizedProvider(provider, cfg)
	llmProvidersMux.Unlock()
	return provider, nil
}

// getOrCreateOptimizedProvider gets or creates an optimized LLM provider
func getOrCreateOptimizedProvider(providerName, modelOverride, reasoningEffort string) (llm.OptimizedProvider, error) {
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}

	re := reasoningEffort
	if re == "" {
		re = cfg.OpenAI.ReasoningEffort
	}

	cacheKey := providerCacheKey{providerName, modelOverride, re}

	llmProvidersMux.RLock()
	if optimizedProvider, exists := optimizedLLMProviders[cacheKey]; exists {
		llmProvidersMux.RUnlock()
		return optimizedProvider, nil
	}
	llmProvidersMux.RUnlock()

	baseProvider, err := getOrCreateProvider(providerName, modelOverride, reasoningEffort)
	if err != nil {
		return nil, err
	}

	llmProvidersMux.Lock()
	defer llmProvidersMux.Unlock()

	if optimizedProvider, exists := optimizedLLMProviders[cacheKey]; exists {
		return optimizedProvider, nil
	}

	optimizedProvider := llm.NewOptimizedProvider(baseProvider, cfg)
	optimizedLLMProviders[cacheKey] = optimizedProvider

	return optimizedProvider, nil
}
