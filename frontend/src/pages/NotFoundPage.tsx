import { FileQuestion } from 'lucide-react'
import { Link } from '../router'
import { EmptyState } from '../components/ui'
import { useI18n } from '../i18n'

export function NotFoundPage() {
  const { t } = useI18n()
  return <div className="page-container"><EmptyState icon={<FileQuestion size={32} />} title={t('notFound.title')} body={t('notFound.body')} action={<Link to="/concepts" className="secondary-button">{t('notFound.back')}</Link>} /></div>
}
