import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const locales = ['en', 'zh']
const localeData = Object.fromEntries(
  locales.map((locale) => [
    locale,
    JSON.parse(
      fs.readFileSync(
        path.join(root, 'src/i18n/locales', `${locale}.json`),
        'utf8'
      )
    ),
  ])
)

function flattenKeys(value, prefix = '', keys = new Set()) {
  for (const [key, child] of Object.entries(value)) {
    const fullKey = prefix ? `${prefix}.${key}` : key
    if (child && typeof child === 'object' && !Array.isArray(child)) {
      flattenKeys(child, fullKey, keys)
    } else {
      keys.add(fullKey)
    }
  }
  return keys
}

function sourceFiles(dir, files = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      sourceFiles(fullPath, files)
    } else if (
      /\.(ts|tsx)$/.test(entry.name) &&
      !/\.test\.(ts|tsx)$/.test(entry.name)
    ) {
      files.push(fullPath)
    }
  }
  return files
}

function collectReferencedKeys() {
  const keys = new Set()
  const patterns = [
    /\b(?:i18n\.)?t\(\s*['"]([^'"`]+)['"]/g,
    // Template literals without interpolation still carry a static key.
    /\b(?:i18n\.)?t\(\s*`([^`$]+)`/g,
  ]
  let dynamicCount = 0

  for (const file of sourceFiles(path.join(root, 'src'))) {
    const source = fs.readFileSync(file, 'utf8')
    for (const pattern of patterns) {
      pattern.lastIndex = 0
      let match
      while ((match = pattern.exec(source))) {
        keys.add(match[1])
      }
    }
    dynamicCount += (source.match(/\b(?:i18n\.)?t\(\s*`[^`]*\$\{/g) ?? [])
      .length
  }

  return { keys, dynamicCount }
}

// i18next plural suffixes are language-specific; only the base key must exist.
function normalizePluralKey(key) {
  return key.replace(/_(?:plural|one|other|two|few|many|zero|\d+)$/, '')
}

const localeKeys = Object.fromEntries(
  locales.map((locale) => [locale, flattenKeys(localeData[locale])])
)
const { keys: referenced, dynamicCount } = collectReferencedKeys()
const errors = []

for (const locale of locales) {
  const missing = [...referenced]
    .filter((key) => !localeKeys[locale].has(key))
    .sort()
  if (missing.length > 0) {
    errors.push(
      `${locale}.json is missing ${missing.length} referenced key(s):\n` +
        missing.map((key) => `  - ${key}`).join('\n')
    )
  }
}

const normalized = Object.fromEntries(
  locales.map((locale) => [
    locale,
    new Set([...localeKeys[locale]].map(normalizePluralKey)),
  ])
)

const onlyEn = [...normalized.en]
  .filter((key) => !normalized.zh.has(key))
  .sort()
const onlyZh = [...normalized.zh]
  .filter((key) => !normalized.en.has(key))
  .sort()

if (onlyEn.length > 0) {
  errors.push(
    `zh.json is missing ${onlyEn.length} key(s) that exist in en.json:\n` +
      onlyEn.map((key) => `  - ${key}`).join('\n')
  )
}

if (onlyZh.length > 0) {
  errors.push(
    `en.json is missing ${onlyZh.length} key(s) that exist in zh.json:\n` +
      onlyZh.map((key) => `  - ${key}`).join('\n')
  )
}

console.log(
  `i18n: ${referenced.size} statically referenced keys, ${localeKeys.en.size} en keys, ${localeKeys.zh.size} zh keys`
)
if (dynamicCount > 0) {
  console.log(
    `i18n: ${dynamicCount} dynamic key expression(s) are not checked statically`
  )
}

if (errors.length > 0) {
  console.error('\n' + errors.join('\n\n'))
  process.exit(1)
}

console.log(
  'i18n: all statically referenced keys exist and locales are in sync'
)
