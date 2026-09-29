import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { LLMForm } from './intelligence/LLMForm'
import { JevForm } from './intelligence/JevForm'
import { DocumentForm } from './intelligence/DocumentForm'

// Intelligence config: LLM distillation + Jev intent gate. Both forms seed from
// the one-shot api.config() payload and save through their own section endpoints.
export default function Intelligence() {
  const { data: config, error, loading, reload } = useApi(api.config, [])

  return (
    <Page>
      <PageHeader title="智能" description="通知提炼、意图判断与附件阅读" />
      {loading || !config ? (
        error ? (
          <InlineError message={error} onRetry={reload} />
        ) : (
          <Loader label="正在加载配置…" />
        )
      ) : (
        <div className="flex flex-col gap-4">
          <LLMForm initial={config.llm} />
          <JevForm initial={config.jev} />
          <DocumentForm initial={config.documents} />
        </div>
      )}
    </Page>
  )
}
