import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'

const repoRoot = execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim()

function read(relativePath) {
  return readFileSync(path.join(repoRoot, relativePath), 'utf8')
}

test('the tracked pre-commit path requires the complete 100% coverage gate', () => {
  const lefthook = read('lefthook.yml')
  const makefile = read('Makefile')
  const trackedHook = read('.githooks/pre-commit')
  const runner = read('scripts/run_required_lefthook.sh')

  assert.match(lefthook, /^pre-commit:\n[\s\S]*?run: make test-coverage$/m)
  assert.match(makefile, /^GO_COVERAGE_MIN \?= 100$/m)
  assert.match(makefile, /^HERMES_COVERAGE_MIN \?= 100$/m)
  assert.match(makefile, /^test-coverage: test-guardrails test-coverage-backend test-coverage-frontend test-coverage-hermes$/m)
  assert.match(trackedHook, /run_required_lefthook\.sh pre-commit --all-files/)
  assert.match(runner, /lefthook run "\$hook_name" "\$@" --no-auto-install/)
})

test('the tracked hook preserves Lefthook success and failure statuses', () => {
  const temporaryDirectory = mkdtempSync(path.join(os.tmpdir(), 'viki-lefthook-contract-'))
  const fakeLefthook = path.join(temporaryDirectory, 'lefthook')
  const invocationLog = path.join(temporaryDirectory, 'invocation.log')
  writeFileSync(fakeLefthook, `#!/bin/sh\nif [ "${'$'}{1:-}" = version ]; then exit 0; fi\nprintf '%s\\n' "$*" > "$VIKI_LEFTHOOK_INVOCATION_LOG"\nexit "${'$'}{VIKI_LEFTHOOK_FAKE_STATUS:-0}"\n`)
  chmodSync(fakeLefthook, 0o755)

  try {
    const environment = {
      ...process.env,
      PATH: `${temporaryDirectory}:${process.env.PATH}`,
      VIKI_LEFTHOOK_INVOCATION_LOG: invocationLog,
    }
    const success = spawnSync('.githooks/pre-commit', [], { cwd: repoRoot, env: environment })
    assert.equal(success.status, 0, success.stderr.toString())
    assert.equal(readFileSync(invocationLog, 'utf8').trim(), 'run pre-commit --all-files --no-auto-install')

    const failure = spawnSync('.githooks/pre-commit', [], {
      cwd: repoRoot,
      env: { ...environment, VIKI_LEFTHOOK_FAKE_STATUS: '23' },
    })
    assert.equal(failure.status, 23)
  } finally {
    rmSync(temporaryDirectory, { recursive: true, force: true })
  }
})

test('GitHub CI runs the same 100% gate on a free standard runner', () => {
  const workflow = read('.github/workflows/coverage.yml')

  assert.match(workflow, /^name: Coverage$/m)
  assert.match(workflow, /^on:\n(?:[\s\S]*?)^  push:\n(?:[\s\S]*?)^  pull_request:\n/m)
  assert.match(workflow, /^permissions:\n  contents: read$/m)
  assert.match(workflow, /^  cancel-in-progress: true$/m)
  assert.match(workflow, /^    runs-on: ubuntu-latest$/m)
  assert.match(workflow, /^    timeout-minutes: 20$/m)
  assert.match(workflow, /^      GO_COVERAGE_MIN: ['"]?100['"]?$/m)
  assert.match(workflow, /^      HERMES_COVERAGE_MIN: ['"]?100['"]?$/m)
  assert.match(workflow, /^      - name: Run 100% coverage gate\n        run: make test-coverage$/m)

  for (const action of ['actions/checkout', 'actions/setup-go', 'actions/setup-node', 'actions/setup-python']) {
    assert.match(workflow, new RegExp(`uses: ${action}@[a-f0-9]{40}`))
  }
  assert.doesNotMatch(workflow, /continue-on-error|\|\|\s*true|upload-artifact|larger-runner/)
})
