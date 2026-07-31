import { FileQuestion } from 'lucide-react'
import { Link } from '../router'
import { EmptyState } from '../components/ui'

export function NotFoundPage() {
  return <div className="page-container"><EmptyState icon={<FileQuestion size={32} />} title="Stránka sa nenašla" body="Odkaz už nemusí byť platný alebo stránka neexistuje." action={<Link to="/primitives" className="secondary-button">Späť na pojmy</Link>} /></div>
}
