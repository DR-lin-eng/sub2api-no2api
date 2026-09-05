type OpenCodeModality = 'text' | 'image' | 'pdf'
type OpenCodeThinkingType = 'adaptive' | 'disable' | 'enabled'

interface OpenCodeModelLimit {
  context: number
  output: number
}

interface OpenCodeModelModalities {
  input: readonly OpenCodeModality[]
  output: readonly OpenCodeModality[]
}

interface OpenCodeThinkingOptions {
  budgetTokens?: number
  type: OpenCodeThinkingType
}

interface OpenCodeModelOptions {
  store?: boolean
  thinking?: OpenCodeThinkingOptions
}

interface OpenCodeModelDefinition {
  name: string
  limit: OpenCodeModelLimit
  modalities?: OpenCodeModelModalities
  options?: OpenCodeModelOptions
  variants?: Readonly<Record<string, Readonly<Record<string, never>>>>
}

export type OpenCodeModelCatalog = Readonly<Record<string, OpenCodeModelDefinition>>

export const openAIModelCatalog = {
  'gpt-6-astra': {
    name: 'GPT-6 Astra',
    limit: {
      context: 1050000,
      output: 128000
    },
    modalities: {
      input: ['text', 'image'],
      output: ['text']
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {},
      max: {}
    }
  },
  'gpt-5.2': {
    name: 'GPT-5.2',
    limit: {
      context: 400000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {}
    }
  },
  'gpt-5.6': {
    name: 'GPT-5.6 (Sol)',
    limit: {
      context: 1050000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {},
      max: {}
    }
  },
  'gpt-5.6-sol': {
    name: 'GPT-5.6 Sol',
    limit: {
      context: 1050000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {},
      max: {}
    }
  },
  'gpt-5.6-terra': {
    name: 'GPT-5.6 Terra',
    limit: {
      context: 1050000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {},
      max: {}
    }
  },
  'gpt-5.6-luna': {
    name: 'GPT-5.6 Luna',
    limit: {
      context: 1050000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {},
      max: {}
    }
  },
  'gpt-5.5': {
    name: 'GPT-5.5',
    limit: {
      context: 1050000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {}
    }
  },
  'gpt-5.4': {
    name: 'GPT-5.4',
    limit: {
      context: 1050000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {}
    }
  },
  'gpt-5.4-mini': {
    name: 'GPT-5.4 Mini',
    limit: {
      context: 400000,
      output: 128000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {}
    }
  },
  'gpt-5.3-codex-spark': {
    name: 'GPT-5.3 Codex Spark',
    limit: {
      context: 128000,
      output: 32000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {},
      xhigh: {}
    }
  },
  'codex-mini-latest': {
    name: 'Codex Mini',
    limit: {
      context: 200000,
      output: 100000
    },
    options: {
      store: false
    },
    variants: {
      low: {},
      medium: {},
      high: {}
    }
  }
} satisfies OpenCodeModelCatalog

export const geminiModelCatalog = {
  'gemini-2.0-flash': {
    name: 'Gemini 2.0 Flash',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    }
  },
  'gemini-2.5-flash': {
    name: 'Gemini 2.5 Flash',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    }
  },
  'gemini-2.5-pro': {
    name: 'Gemini 2.5 Pro',
    limit: {
      context: 2097152,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-3.5-flash': {
    name: 'Gemini 3.5 Flash',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    }
  },
  'gemini-3-flash-preview': {
    name: 'Gemini 3 Flash Preview',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    }
  },
  'gemini-3-pro-preview': {
    name: 'Gemini 3 Pro Preview',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-3.1-pro-preview': {
    name: 'Gemini 3.1 Pro Preview',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  }
} satisfies OpenCodeModelCatalog

export const antigravityGeminiModelCatalog = {
  'gemini-2.5-flash': {
    name: 'Gemini 2.5 Flash',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'disable'
      }
    }
  },
  'gemini-2.5-flash-lite': {
    name: 'Gemini 2.5 Flash Lite',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-2.5-flash-thinking': {
    name: 'Gemini 2.5 Flash (Thinking)',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-3-flash': {
    name: 'Gemini 3 Flash',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-3.1-pro-low': {
    name: 'Gemini 3.1 Pro Low',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-3.1-pro-high': {
    name: 'Gemini 3.1 Pro High',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-2.5-flash-image': {
    name: 'Gemini 2.5 Flash Image',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image'],
      output: ['image']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'gemini-3.1-flash-image': {
    name: 'Gemini 3.1 Flash Image',
    limit: {
      context: 1048576,
      output: 65536
    },
    modalities: {
      input: ['text', 'image'],
      output: ['image']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  }
} satisfies OpenCodeModelCatalog

export const claudeModelCatalog = {
  'claude-fable-5-1': {
    name: 'Claude Fable 5.1',
    limit: {
      context: 1048576,
      output: 128000
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        type: 'adaptive'
      }
    }
  },
  'claude-fable-5': {
    name: 'Claude Fable 5',
    limit: {
      context: 1048576,
      output: 128000
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        type: 'adaptive'
      }
    }
  },
  'claude-opus-4-6-thinking': {
    name: 'Claude 4.6 Opus (Thinking)',
    limit: {
      context: 200000,
      output: 128000
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  },
  'claude-sonnet-4-6': {
    name: 'Claude 4.6 Sonnet',
    limit: {
      context: 200000,
      output: 64000
    },
    modalities: {
      input: ['text', 'image', 'pdf'],
      output: ['text']
    },
    options: {
      thinking: {
        budgetTokens: 24576,
        type: 'enabled'
      }
    }
  }
} satisfies OpenCodeModelCatalog

export const grokModelCatalog = {
  'grok-4.5': {
    name: 'Grok 4.5',
    limit: { context: 1000000, output: 128000 }
  },
  'grok-4.3': {
    name: 'Grok 4.3',
    limit: { context: 1000000, output: 128000 }
  },
  'grok-build-0.1': {
    name: 'Grok Build 0.1',
    limit: { context: 256000, output: 128000 }
  },
  'grok-composer-2.5-fast': {
    name: 'Grok Composer 2.5 Fast',
    limit: { context: 500000, output: 128000 }
  }
} satisfies OpenCodeModelCatalog
