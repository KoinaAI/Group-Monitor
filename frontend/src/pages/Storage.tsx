import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { BackupForm } from './storage/BackupForm'

export default function Storage() {
  const { data: config, error, loading, reload } = useApi(api.config, [])
  return (
    <Page>
      <PageHeader title="存储与备份" description="只保留通过意图判断的正式通知，并按计划备份到你的存储桶" />
      {loading || !config ? (
        error ? <InlineError message={error} onRetry={reload} /> : <Loader label="正在加载配置…" />
      ) : <BackupForm initial={config.backup} />}
    </Page>
  )
}
