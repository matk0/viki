import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { generateMessages } from '@cucumber/gherkin'
import { IdGenerator, SourceMediaType } from '@cucumber/messages'

const root = fileURLToPath(new URL('../../', import.meta.url))
const directory = new URL('../../specs/viki/', import.meta.url)
const ids = new Set()
const documents = []
const identify = (id) => {
  if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(id) || ids.has(id)) throw new Error('Invalid or duplicate source ID: ' + id)
  ids.add(id)
  return id
}
const tags = (values, feature) => {
  const identifiers = values.filter((tag) => tag.name.startsWith('@id:'))
  if (identifiers.length !== 1 || values.some((tag) => !tag.name.startsWith('@id:') && !(feature && tag.name.startsWith('@concept:')))) throw new Error('Unsupported source tags')
  return { id: identify(identifiers[0].name.slice(4)), references: values.filter((tag) => tag.name.startsWith('@concept:')).map((tag) => tag.name.slice(9)) }
}
for (const name of (await readdir(new URL('concepts/', directory))).sort()) {
  if (!name.endsWith('.md')) continue
  const content = await readFile(new URL('concepts/' + name, directory), 'utf8')
  const header = content.match(/^<!-- business-id: ([A-Za-z0-9_.-]+) -->\n/)
  const title = content.match(/^# (.+)$/m)
  if (!header || !title) throw new Error('Missing concept identity/title: ' + name)
  documents.push({
    id: identify(header[1]), title: title[1], kind: 'concept', body: content,
    steps: [], references: (content.match(/^<!-- relates-to: (.+) -->$/m)?.[1] ?? '').split(', ').filter(Boolean),
    path: 'specs/viki/concepts/' + name,
  })
}
for (const name of (await readdir(directory)).sort()) {
  if (!name.endsWith('.feature')) continue
  const content = await readFile(new URL(name, directory), 'utf8')
  const envelopes = generateMessages(content, 'specs/viki/' + name, SourceMediaType.TEXT_X_CUCUMBER_GHERKIN_PLAIN, {
    newId: IdGenerator.uuid(), defaultDialect: 'en', includeSource: false, includeGherkinDocument: true, includePickles: false,
  })
  const errors = envelopes.filter((envelope) => envelope.parseError)
  const feature = envelopes.find((envelope) => envelope.gherkinDocument)?.gherkinDocument.feature
  if (errors.length || !feature || feature.language !== 'en') throw new Error('Invalid English Gherkin: ' + name)
  const metadata = tags(feature.tags, true)
  const path = 'specs/viki/' + name
  documents.push({ ...metadata, kind: 'feature', title: feature.name, body: feature.description, steps: [], path })
  for (const child of feature.children) {
    const scenario = child.scenario
    if (!scenario || scenario.examples.length || scenario.keyword !== 'Scenario') throw new Error('Unsupported Gherkin construct: ' + name)
    const steps = scenario.steps.map((step) => {
      const keyword = step.keyword.trim()
      if (step.dataTable || step.docString || !['Given', 'When', 'Then', 'And', 'But'].includes(keyword)) throw new Error('Unsupported step: ' + name)
      return { keyword: keyword.toLowerCase(), text: step.text }
    })
    if (steps[0]?.keyword !== 'given' || !steps.some((step) => step.keyword === 'when') || !steps.some((step) => step.keyword === 'then')) throw new Error('Scenario needs Given, When, Then')
    documents.push({ ...tags(scenario.tags, false), references: metadata.references, kind: 'scenario', parent: metadata.id, title: scenario.name, body: scenario.description, steps, path })
  }
}
for (const document of documents) {
  for (const reference of document.references) {
    if (!documents.some((candidate) => candidate.id === reference && candidate.kind === 'concept')) throw new Error('Unknown concept: ' + reference)
  }
}
const hash = (value) => createHash('sha256').update(value).digest('hex')
const uuid = (value) => {
  const digest = hash(value)
  return [digest.slice(0, 8), digest.slice(8, 12), '5' + digest.slice(13, 16), 'a' + digest.slice(17, 20), digest.slice(20, 32)].join('-')
}
const commit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim()
const createdAt = execFileSync('git', ['show', '-s', '--format=%cI', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim()
const author = { id: uuid('repository'), displayName: 'Repository snapshot', email: '', createdAt }
const pages = documents.map((document) => {
  const pageId = uuid(document.id)
  const revisionId = uuid(JSON.stringify(document))
  const page = {
    id: pageId, kind: document.kind, parentId: document.parent ? uuid(document.parent) : undefined,
    slug: document.id.toLowerCase(), title: document.title, repositoryRevisionId: revisionId,
    approved: false, hasDraft: false, unresolvedObjections: 0, createdAt, updatedAt: createdAt,
  }
  const revision = {
    id: revisionId, pageId, title: document.title, bodyMd: document.body, steps: document.steps,
    references: document.references.map((id) => ({ targetPageId: uuid(id), relation: 'relates_to', targetTitle: documents.find((value) => value.id === id).title })),
    number: 1, status: 'repository_baseline', createdBy: author, createdAt,
  }
  return {
    sourceId: document.id, sourcePath: document.path, page, repositoryRevision: revision, revisions: [revision],
    comments: [], objections: [], children: [], reviewStates: [],
  }
})
for (const detail of pages) detail.children = pages.filter((other) => other.page.parentId === detail.page.id).map((other) => other.page)
const catalogue = {
  source: { repository: 'https://github.com/matk0/viki', commit, createdAt, executionEvidence: false },
  pages,
}
await mkdir(new URL('../src/', import.meta.url), { recursive: true })
await writeFile(new URL('../src/catalogue.json', import.meta.url), JSON.stringify(catalogue, null, 2) + '\n')
console.log('Bundled ' + pages.length + ' explanatory pages from ' + commit.slice(0, 7) + '; no Cucumber execution claim.')
