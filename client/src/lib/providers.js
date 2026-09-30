// The AI providers a person can bring a key for; ids match the backend.
export const PROVIDERS = [
  { id: 'anthropic', name: 'Anthropic', blurb: 'Use Claude models for generation.', placeholder: 'sk-ant-...' },
  { id: 'openai', name: 'Codex (OpenAI)', blurb: 'Use GPT and Codex models.', placeholder: 'sk-proj-...' },
  { id: 'xai', name: 'Grok (xAI)', blurb: 'Use Grok models for generation.', placeholder: 'xai-...' },
]

// providerName is how a provider is named in one line, for buttons and messages.
export const providerName = (id) => (PROVIDERS.find((p) => p.id === id) || { name: id }).name

// generationLabel names the model family a key unlocks, as the backend does in its progress steps.
const FAMILIES = { anthropic: 'Claude (Anthropic)', openai: 'GPT (OpenAI)', xai: 'Grok (xAI)' }
export const generationLabel = (id) => FAMILIES[id] || id
