import React, {createContext, useEffect, useState} from "react"
import getConfig from "next/config"

interface RuntimeConfig {
  flowAccountAddress: string
  flowAccountPrivateKey: string
  flowAccountPublicKey: string
  flowAccountKeyId: string
  flowAccessNode: string
}

const {publicRuntimeConfig} = getConfig()

const defaultConfig = {
  flowAccountAddress: publicRuntimeConfig.flowAccountAddress || "f8d6e0586b0a20c7",
  flowAccountPrivateKey: publicRuntimeConfig.flowAccountPrivateKey || "f8e188e8af0b8b414be59c4a1a15cc666c898fb34d94156e9b51e18bfde754a5",
  flowAccountPublicKey: publicRuntimeConfig.flowAccountPublicKey || "6e70492cb4ec2a6013e916114bc8bf6496f3335562f315e18b085c19da659bdfd88979a5904ae8bd9b4fd52a07fc759bad9551c04f289210784e7b08980516d2",
  flowAccountKeyId: publicRuntimeConfig.flowAccountKeyId || "0",
  flowAccessNode: publicRuntimeConfig.flowAccessNode || "http://localhost:8888",
}

export const ConfigContext = createContext<RuntimeConfig>(defaultConfig)

export async function fetchConfigFromAPI(): Promise<RuntimeConfig> {
  if (publicRuntimeConfig.isLocal) {
    return defaultConfig
  }

  return fetch("http://localhost:8701/api/")
    .then(res => res.json())
    .catch(e => {
      console.log(
        `Warning: Failed to fetch config from API. 
         If you see this warning during CI you can ignore it.
         Returning default config.
         ${e}
          `
      )
      return defaultConfig
    })
}

export function ConfigContextProvider({children}: {children: React.ReactNode}) {
  const [config, setConfig] = useState<RuntimeConfig>()

  useEffect(() => {
    async function fetchConfig() {
      const config = await fetchConfigFromAPI()
      setConfig(config)
    }

    fetchConfig()
  }, [])

  if (!config) return null

  return (
    <ConfigContext.Provider value={config}>{children}</ConfigContext.Provider>
  )
}
