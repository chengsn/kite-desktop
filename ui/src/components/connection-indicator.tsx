import { RefreshCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export const ConnectionIndicator: React.FC<{
  isConnected: boolean
  onReconnect?: () => void
  children?: React.ReactNode
}> = ({ isConnected, onReconnect, children }) => {
  const { t } = useTranslation()

  if (isConnected) {
    return (
      <div className="flex items-center gap-1.5">
        <div className="w-2.5 h-2.5 rounded-full bg-green-400 breathing-indicator" />
        {children}
      </div>
    )
  } else {
    return (
      <div className="flex items-center gap-1.5">
        <div className="w-2.5 h-2.5 rounded-full bg-red-400 breathing-indicator" />
        {children}
        {onReconnect && (
          <button
            type="button"
            onClick={onReconnect}
            aria-label={t('common.reconnect')}
            className="rounded-full p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <RefreshCcw className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    )
  }
}
