import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from 'react'

export type Locale = 'sk' | 'en'

const sk = {
  'language.label': 'Jazyk',
  'language.slovak': 'Slovenčina',
  'language.english': 'English',
  'common.loading': 'Načítavam…',
  'common.close': 'Zavrieť',
  'common.cancel': 'Zrušiť',
  'common.system': 'Systém',
  'common.create': 'Vytvoriť',
  'common.edit': 'Upraviť',
  'common.saveFailed': 'Akciu sa nepodarilo dokončiť.',
  'status.approved': 'Schválené',
  'status.superseded': 'Nahradené',
  'status.draft': 'Draft',
  'kind.concept': 'Koncept',
  'kind.feature': 'Funkcia',
  'kind.scenario': 'Scenár',
  'kind.concepts': 'Koncepty',
  'kind.features': 'Funkcie',
  'kind.scenarios': 'Scenáre',
  'nav.open': 'Otvoriť navigáciu',
  'nav.close': 'Zavrieť navigáciu',
  'nav.main': 'Hlavná navigácia',
  'nav.search': 'Hľadať',
  'nav.audit': 'História zmien',
  'nav.logout': 'Odhlásiť',
  'assistant.open': 'Otvoriť asistenta',
  'assistant.close': 'Zavrieť asistenta',
  'app.loading': 'Načítavam viki…',
  'login.welcome': 'Vitajte späť',
  'login.title': 'Prihlásenie do pracovného priestoru',
  'login.email': 'E-mail',
  'login.password': 'Heslo',
  'login.submit': 'Prihlásiť sa',
  'login.submitting': 'Prihlasujem…',
  'login.failed': 'Prihlásenie sa nepodarilo.',
  'library.conceptsDescription': 'Kanonické podstatné mená a slovesá používané vo firme.',
  'library.featuresDescription': 'Funkcie systému a ich konkrétne Gherkin scenáre.',
  'library.addConcept': 'Pridať koncept',
  'library.addFeature': 'Pridať funkciu',
  'library.searchConcepts': 'Hľadať v konceptoch…',
  'library.searchFeatures': 'Hľadať vo funkciách…',
  'library.none': 'Nič sa nenašlo',
  'library.noneBody': 'Skúste zmeniť vyhľadávanie alebo filter.',
  'library.filterLabel': 'Filtrovať podľa stavu: {status}',
  'library.filterList': 'Filtrovať podľa stavu',
  'library.all': 'Všetky',
  'library.approved': 'Schválené',
  'library.draft': 'Draft',
  'library.nouns': 'Podstatné mená',
  'library.verbs': 'Slovesá',
  'library.scenarioCount.one': '1 scenár',
  'library.scenarioCount.other': '{count} scenárov',
  'page.loadFailed': 'Stránku sa nepodarilo načítať.',
  'page.loading': 'Načítavam stránku…',
  'page.approved': 'Schválené',
  'page.draftNumber': 'Draft #{number}',
  'page.revisionMeta': 'Verzia #{number} · {author} · {date}',
  'page.emptyDescription': '_Táto stránka zatiaľ nemá opis._',
  'page.related': 'Súvisiace stránky',
  'page.scenarios': 'Scenáre',
  'page.addScenario': 'Pridať scenár',
  'page.noScenarios': 'Táto funkcia zatiaľ nemá scenáre.',
  'page.versionUnavailable': 'Verzia nie je dostupná',
  'page.versionUnavailableBody': 'Vyberte dostupnú schválenú verziu alebo draft.',
  'page.history': 'História',
  'page.newVersion': 'Nová verzia',
  'page.editDraft': 'Zmeniť',
  'page.review': 'Kontrola',
  'development.queued': 'Čaká na vývoj',
  'development.running': 'Vo vývoji',
  'development.developed': 'Vyvinuté',
  'development.blocked': 'Vyžaduje zásah',
  'relation.uses': 'používa',
  'relation.requires': 'vyžaduje',
  'relation.produces': 'vytvára',
  'relation.relates_to': 'súvisí s',
  'search.title': 'Vyhľadávanie',
  'search.description': 'Nájdite schválené definície, funkcie, scenáre aj rozpracované drafty.',
  'search.placeholder': 'Čo hľadáte?',
  'search.pageType': 'Typ stránky',
  'search.pageTypes': 'Typy stránok',
  'search.allTypes': 'Všetky typy',
  'search.includeDrafts': 'Zahrnúť drafty',
  'search.submit': 'Hľadať',
  'search.searching': 'Hľadám v pracovnom priestore…',
  'search.noResults': 'Žiadne výsledky',
  'search.noResultsBody': 'Skúste všeobecnejší výraz alebo zahrňte drafty.',
  'search.noPreview': 'Bez textového náhľadu.',
  'audit.title': 'História zmien',
  'audit.description': 'Nemenný záznam ľudských aj asistenčných akcií v pracovnom priestore.',
  'audit.unavailable': 'História sa nedá načítať',
  'audit.pageCreated': 'vytvoril(a) stránku',
  'audit.revisionSaved': 'uložil(a) nový draft',
  'audit.revisionApproved': 'schválil(a) verziu',
  'audit.revisionPublished': 'publikoval(a) verziu',
  'audit.commentCreated': 'pridal(a) komentár',
  'audit.objectionCreated': 'vzniesol/vzniesla námietku',
  'audit.objectionResolved': 'vyriešil(a) námietku',
  'audit.assistantDrafts': 'vytvoril(a) drafty cez asistenta',
  'audit.assistantProposal': 'pripravil(a) návrh cez asistenta',
  'audit.assistantPublished': 'schválil(a) návrh asistenta',
  'audit.assistantDiscarded': 'odmietol/odmietla návrh asistenta',
  'notFound.title': 'Stránka sa nenašla',
  'notFound.body': 'Odkaz už nemusí byť platný alebo stránka neexistuje.',
  'notFound.back': 'Späť na koncepty',
  'new.eyebrow': 'Nový draft',
  'new.createConcept': 'Vytvoriť koncept',
  'new.createFeature': 'Vytvoriť funkciu',
  'new.createScenario': 'Vytvoriť scenár',
  'new.conceptKind': 'Druh konceptu',
  'new.conceptKinds': 'Druhy konceptov',
  'new.noun': 'Podstatné meno',
  'new.verb': 'Sloveso',
  'new.title': 'Názov',
  'new.initialScenario': 'Prvý scenár',
  'new.scenarioTitle': 'Názov scenára',
  'new.scenarioSlug': 'Slug scenára',
  'new.titlePlaceholder': 'Napríklad Dostupnosť služby',
  'new.slugHelp': 'Malé písmená bez diakritiky, oddelené pomlčkou.',
  'new.failed': 'Stránku sa nepodarilo vytvoriť.',
  'new.creating': 'Vytváram…',
  'new.submit': 'Vytvoriť draft',
  'bdd.heading': 'Kroky správania',
  'bdd.structured': 'Štruktúrovaný Gherkin',
  'bdd.keyword': 'Kľúčové slovo kroku {number}',
  'bdd.keywords': 'Kľúčové slová kroku {number}',
  'bdd.text': 'Text kroku {number}',
  'bdd.placeholder': 'Popíšte podmienku alebo výsledok…',
  'bdd.searchDefinition': 'Vyhľadať definíciu kroku {number}',
  'bdd.searchPlaceholder': 'Začnite písať a vyberte existujúci krok…',
  'bdd.proposeDefinition': 'Navrhnúť nový krok',
  'bdd.newDefinition': 'Nová definícia kroku {number}',
  'bdd.changeDefinition': 'Zmeniť',
  'bdd.useExisting': 'Použiť existujúci krok',
  'bdd.parameter': 'Parameter {parameter} kroku {number}',
  'bdd.used.one': 'Použité v 1 scenári',
  'bdd.used.other': 'Použité v {count} scenároch',
  'bdd.noDefinitions': 'Žiadny zodpovedajúci krok',
  'bdd.moveUp': 'Posunúť krok hore',
  'bdd.moveDown': 'Posunúť krok dole',
  'bdd.remove': 'Odstrániť krok',
  'bdd.add': 'Pridať krok',
  'bdd.given': 'Pokiaľ',
  'bdd.when': 'Keď',
  'bdd.then': 'Potom',
  'bdd.and': 'A zároveň',
  'bdd.but': 'Ale',
  'markdown.toolbar': 'Formátovanie Markdown',
  'markdown.bold': 'Tučné',
  'markdown.italic': 'Kurzíva',
  'markdown.heading': 'Nadpis',
  'markdown.list': 'Zoznam',
  'markdown.content': 'Obsah stránky',
  'markdown.placeholder': 'Napíšte zrozumiteľný popis…',
  'editor.conflict': 'Medzitým vznikla novšia verzia. Obnovte stránku a zapracujte svoje zmeny znova.',
  'editor.failed': 'Draft sa nepodarilo uložiť.',
  'editor.related': 'Súvisiace stránky',
  'editor.structuredRelations': 'Štruktúrované vzťahy',
  'editor.relationTarget': 'Cieľ vzťahu {number}',
  'editor.relationTargets': 'Ciele vzťahu {number}',
  'editor.relationType': 'Typ vzťahu {number}',
  'editor.relationPlaceholder': 'napr. používa',
  'editor.removeRelation': 'Odstrániť vzťah',
  'editor.addRelation': 'Pridať vzťah',
  'editor.saving': 'Ukladám…',
  'editor.save': 'Uložiť novú verziu',
  'history.eyebrow': 'Nemenná auditná stopa',
  'history.title': 'História verzií',
  'history.revision': 'Verzia #{number}',
  'history.comparing': 'Porovnávate so schválenou verziou #{number}',
  'history.selected': 'Vybraná verzia',
  'history.approved': 'Schválená verzia',
  'history.noDescription': '_Bez opisu_',
  'review.failed': 'Akciu sa nepodarilo dokončiť.',
  'review.statuses': 'Možné stavy',
  'review.statusBlocked': 'Zablokované',
  'review.statusDraft': 'Draft',
  'review.revision': 'Kontrola verzie #{number}',
  'review.draftApproval': 'Schválenie draftu',
  'review.raiseObjection': 'Vzniesť námietku',
  'review.approveDraft': 'Schváliť',
  'review.blockers': 'Čo blokuje schválenie',
  'review.objections': 'Námietky',
  'review.resolved': 'Vyriešená',
  'review.reason': 'Dôvod námietky',
  'review.reasonPlaceholder': 'Čo treba opraviť?',
  'review.submitObjection': 'Odoslať námietku',
  'review.parentFeatureRequired': 'Najprv schváľte funkciu',
  'review.parentFeatureRequiredBody': 'Scenár možno schváliť až po schválení funkcie „{title}“.',
  'review.comments': 'Komentáre',
  'review.discussion': 'Diskusia ({count})',
  'review.description': 'Opis',
  'review.commentLabel': 'Komentár',
  'review.addComment': 'Pridať komentár',
  'review.reply': 'Odpovedať',
  'review.markResolved': 'Označiť ako vyriešené',
  'review.replyPlaceholder': 'Napíšte odpoveď…',
  'review.send': 'Odoslať',
  'assistant.name': 'viki asistent',
  'assistant.subtitle': 'Odpovede a úpravy',
  'assistant.newConversation': 'Nový rozhovor',
  'assistant.qa': 'Otázky',
  'assistant.edit': 'Úpravy',
  'assistant.unavailable': 'Asistent je momentálne nedostupný.',
  'assistant.coreAvailable': 'Viki môžete naďalej používať bez asistenta.',
  'assistant.checkConnection': 'Skontrolovať spojenie',
  'assistant.preparing': 'Pripravujem asistenta…',
  'assistant.waiting': 'Asistent čaká na spojenie',
  'assistant.waitingBody': 'Keď bude asistent dostupný, môžete začať nový rozhovor. Koncepty, funkcie a scenáre zostávajú dostupné.',
  'assistant.needClarification': 'Potrebujem doplnenie',
  'assistant.yourAnswer': 'Vaša odpoveď',
  'assistant.continue': 'Pokračovať',
  'assistant.askPlaceholder': 'Opýtajte sa viki…',
  'assistant.editPlaceholder': 'Opíšte, čo má viki pridať alebo zmeniť…',
  'assistant.listening': 'Počúvam po slovensky…',
  'assistant.shortcut': '⌘⇧M diktuje · Enter odosiela',
  'assistant.stop': 'Zastaviť',
  'assistant.stopVoice': 'Zastaviť hlasový vstup',
  'assistant.startVoice': 'Začať hlasový vstup',
  'assistant.voiceTitle': 'Hlasový vstup v slovenčine (⌘⇧M / Ctrl+Shift+M)',
  'assistant.voiceUnsupported': 'Tento prehliadač nepodporuje hlasový vstup',
  'assistant.send': 'Odoslať',
  'assistant.conversation': 'Rozhovor: {title}',
  'assistant.conversations': 'Rozhovory',
  'assistant.welcome': 'Čo potrebujete zachytiť?',
  'assistant.welcomeBody': 'Opýtajte sa na firemné pravidlá alebo opíšte nový koncept, funkciu či scenár.',
  'assistant.you': 'Vy',
  'assistant.revision': 'verzia {id}',
  'assistant.exactRevision': 'Presná verzia: {id}',
  'assistant.draftCreated': 'Draft vytvorený',
  'assistant.disconnected': 'Spojenie s asistentom sa prerušilo.',
  'assistant.reconnecting': 'Spojenie s asistentom sa obnovuje…',
  'assistant.reconnect': 'Pripojiť znova',
  'assistant.conversationDated': 'Rozhovor · {date}',
  'assistant.conversationNumber': 'Rozhovor {number}',
  'assistant.activity.searching': 'Hľadám vo viki…',
  'assistant.activity.drafting': 'Vytváram drafty…',
  'assistant.activity.awaiting': 'Drafty čakajú na schválenie…',
  'assistant.activity.clarifying': 'Čakám na doplnenie…',
  'assistant.activity.stopping': 'Zastavujem…',
  'assistant.activity.submitting': 'Odosielam…',
  'assistant.activity.editing': 'Premýšľam nad úpravou…',
  'assistant.activity.answering': 'Hľadám odpoveď…',
  'assistant.loadConversationsFailed': 'Rozhovory sa nepodarilo načítať.',
  'assistant.loadConversationFailed': 'Rozhovor sa nepodarilo načítať.',
  'assistant.createConversationFailed': 'Nový rozhovor sa nepodarilo vytvoriť.',
  'assistant.managementForbidden': 'Príkazy na správu asistenta nie sú vo viki povolené.',
  'assistant.sendFailed': 'Správu sa nepodarilo odoslať.',
  'assistant.stopFailed': 'Asistenta sa nepodarilo zastaviť.',
  'assistant.clarificationFailed': 'Doplnenie sa nepodarilo odoslať.',
  'voice.notAllowed': 'Povoľte prístup k mikrofónu a skúste to znova.',
  'voice.unavailable': 'Mikrofón nie je dostupný.',
  'voice.noSpeech': 'Nezachytil som žiadnu reč. Skúste to znova.',
  'voice.unrecognized': 'Hlasový vstup sa nepodarilo rozpoznať.',
  'voice.unsupported': 'Tento prehliadač nepodporuje hlasový vstup.',
  'voice.startFailed': 'Hlasový vstup sa nepodarilo spustiť.',
} as const

export type TranslationKey = keyof typeof sk
export type Translate = (key: TranslationKey, values?: Record<string, string | number>) => string

const en: Record<TranslationKey, string> = {
  'language.label': 'Language', 'language.slovak': 'Slovenčina', 'language.english': 'English',
  'common.loading': 'Loading…', 'common.close': 'Close', 'common.cancel': 'Cancel', 'common.system': 'System', 'common.create': 'Create', 'common.edit': 'Edit', 'common.saveFailed': 'The action could not be completed.',
  'status.approved': 'Approved', 'status.superseded': 'Superseded', 'status.draft': 'Draft',
  'kind.concept': 'Concept', 'kind.feature': 'Feature', 'kind.scenario': 'Scenario', 'kind.concepts': 'Concepts', 'kind.features': 'Features', 'kind.scenarios': 'Scenarios',
  'nav.open': 'Open navigation', 'nav.close': 'Close navigation', 'nav.main': 'Main navigation', 'nav.search': 'Search', 'nav.audit': 'Change history', 'nav.logout': 'Log out',
  'assistant.open': 'Open assistant', 'assistant.close': 'Close assistant', 'app.loading': 'Loading viki…',
  'login.welcome': 'Welcome back', 'login.title': 'Sign in to your workspace', 'login.email': 'Email', 'login.password': 'Password', 'login.submit': 'Sign in', 'login.submitting': 'Signing in…', 'login.failed': 'Sign-in failed.',
  'library.conceptsDescription': 'Canonical nouns and verbs used by the company.', 'library.featuresDescription': 'System features and their concrete Gherkin scenarios.', 'library.addConcept': 'Add concept', 'library.addFeature': 'Add feature', 'library.searchConcepts': 'Search concepts…', 'library.searchFeatures': 'Search features…', 'library.none': 'Nothing found', 'library.noneBody': 'Try changing the search or filter.', 'library.filterLabel': 'Filter by status: {status}', 'library.filterList': 'Filter by status', 'library.all': 'All', 'library.approved': 'Approved', 'library.draft': 'Draft', 'library.nouns': 'Nouns', 'library.verbs': 'Verbs', 'library.scenarioCount.one': '1 scenario', 'library.scenarioCount.other': '{count} scenarios',
  'page.loadFailed': 'The page could not be loaded.', 'page.loading': 'Loading page…', 'page.approved': 'Approved', 'page.draftNumber': 'Draft #{number}', 'page.revisionMeta': 'Version #{number} · {author} · {date}', 'page.emptyDescription': '_This page does not have a description yet._', 'page.related': 'Related pages', 'page.scenarios': 'Scenarios', 'page.addScenario': 'Add scenario', 'page.noScenarios': 'This feature does not have any scenarios yet.', 'page.versionUnavailable': 'Version unavailable', 'page.versionUnavailableBody': 'Select an available approved version or draft.', 'page.history': 'History', 'page.newVersion': 'New version', 'page.editDraft': 'Edit', 'page.review': 'Review', 'development.queued': 'Waiting for development', 'development.running': 'In development', 'development.developed': 'Developed', 'development.blocked': 'Needs attention', 'relation.uses': 'uses', 'relation.requires': 'requires', 'relation.produces': 'produces', 'relation.relates_to': 'relates to',
  'search.title': 'Search', 'search.description': 'Find approved definitions, features, scenarios, and work-in-progress drafts.', 'search.placeholder': 'What are you looking for?', 'search.pageType': 'Page type', 'search.pageTypes': 'Page types', 'search.allTypes': 'All types', 'search.includeDrafts': 'Include drafts', 'search.submit': 'Search', 'search.searching': 'Searching the workspace…', 'search.noResults': 'No results', 'search.noResultsBody': 'Try a broader query or include drafts.', 'search.noPreview': 'No text preview.',
  'audit.title': 'Change history', 'audit.description': 'An immutable record of human and assistant actions in the workspace.', 'audit.unavailable': 'History cannot be loaded', 'audit.pageCreated': 'created a page', 'audit.revisionSaved': 'saved a new draft', 'audit.revisionApproved': 'approved a version', 'audit.revisionPublished': 'published a version', 'audit.commentCreated': 'added a comment', 'audit.objectionCreated': 'raised an objection', 'audit.objectionResolved': 'resolved an objection', 'audit.assistantDrafts': 'created drafts through the assistant', 'audit.assistantProposal': 'prepared an assistant proposal', 'audit.assistantPublished': 'approved an assistant proposal', 'audit.assistantDiscarded': 'rejected an assistant proposal',
  'notFound.title': 'Page not found', 'notFound.body': 'The link may no longer be valid or the page does not exist.', 'notFound.back': 'Back to concepts',
  'new.eyebrow': 'New draft', 'new.createConcept': 'Create concept', 'new.createFeature': 'Create feature', 'new.createScenario': 'Create scenario', 'new.conceptKind': 'Concept kind', 'new.conceptKinds': 'Concept kinds', 'new.noun': 'Noun', 'new.verb': 'Verb', 'new.title': 'Title', 'new.initialScenario': 'First scenario', 'new.scenarioTitle': 'Scenario title', 'new.scenarioSlug': 'Scenario slug', 'new.titlePlaceholder': 'For example Service availability', 'new.slugHelp': 'Lowercase ASCII letters separated by hyphens.', 'new.failed': 'The page could not be created.', 'new.creating': 'Creating…', 'new.submit': 'Create draft',
  'bdd.heading': 'Behavior steps', 'bdd.structured': 'Structured Gherkin', 'bdd.keyword': 'Step {number} keyword', 'bdd.keywords': 'Step {number} keywords', 'bdd.text': 'Step {number} text', 'bdd.placeholder': 'Describe the condition or outcome…', 'bdd.searchDefinition': 'Search step definition {number}', 'bdd.searchPlaceholder': 'Start typing and choose an existing step…', 'bdd.proposeDefinition': 'Propose a new step', 'bdd.newDefinition': 'New step definition {number}', 'bdd.changeDefinition': 'Change', 'bdd.useExisting': 'Use an existing step', 'bdd.parameter': 'Parameter {parameter} of step {number}', 'bdd.used.one': 'Used in 1 scenario', 'bdd.used.other': 'Used in {count} scenarios', 'bdd.noDefinitions': 'No matching step', 'bdd.moveUp': 'Move step up', 'bdd.moveDown': 'Move step down', 'bdd.remove': 'Remove step', 'bdd.add': 'Add step', 'bdd.given': 'Given', 'bdd.when': 'When', 'bdd.then': 'Then', 'bdd.and': 'And', 'bdd.but': 'But',
  'markdown.toolbar': 'Markdown formatting', 'markdown.bold': 'Bold', 'markdown.italic': 'Italic', 'markdown.heading': 'Heading', 'markdown.list': 'List', 'markdown.content': 'Page content', 'markdown.placeholder': 'Write a clear description…',
  'editor.conflict': 'A newer version was created in the meantime. Refresh the page and apply your changes again.', 'editor.failed': 'The draft could not be saved.', 'editor.related': 'Related pages', 'editor.structuredRelations': 'Structured relationships', 'editor.relationTarget': 'Relationship {number} target', 'editor.relationTargets': 'Relationship {number} targets', 'editor.relationType': 'Relationship {number} type', 'editor.relationPlaceholder': 'e.g. uses', 'editor.removeRelation': 'Remove relationship', 'editor.addRelation': 'Add relationship', 'editor.saving': 'Saving…', 'editor.save': 'Save new version',
  'history.eyebrow': 'Immutable audit trail', 'history.title': 'Version history', 'history.revision': 'Version #{number}', 'history.comparing': 'Comparing with approved version #{number}', 'history.selected': 'Selected version', 'history.approved': 'Approved version', 'history.noDescription': '_No description_',
  'review.failed': 'The action could not be completed.', 'review.statuses': 'Possible statuses', 'review.statusBlocked': 'Blocked', 'review.statusDraft': 'Draft', 'review.revision': 'Review version #{number}', 'review.draftApproval': 'Draft approval', 'review.raiseObjection': 'Raise an objection', 'review.approveDraft': 'Approve draft', 'review.blockers': 'What blocks approval', 'review.objections': 'Objections', 'review.resolved': 'Resolved', 'review.reason': 'Reason for objection', 'review.reasonPlaceholder': 'What needs to change?', 'review.submitObjection': 'Submit objection', 'review.parentFeatureRequired': 'Approve the parent feature first', 'review.parentFeatureRequiredBody': 'This scenario can be approved only after the feature “{title}” is approved.', 'review.comments': 'Comments', 'review.discussion': 'Discussion ({count})', 'review.description': 'Description', 'review.commentLabel': 'Comment', 'review.addComment': 'Add comment', 'review.reply': 'Reply', 'review.markResolved': 'Mark as resolved', 'review.replyPlaceholder': 'Write a reply…', 'review.send': 'Send',
  'assistant.name': 'viki assistant', 'assistant.subtitle': 'Answers and edits', 'assistant.newConversation': 'New conversation', 'assistant.qa': 'Questions', 'assistant.edit': 'Edit', 'assistant.unavailable': 'The assistant is currently unavailable.', 'assistant.coreAvailable': 'You can continue using Viki without the assistant.', 'assistant.checkConnection': 'Check connection', 'assistant.preparing': 'Preparing assistant…', 'assistant.waiting': 'Assistant is waiting for a connection', 'assistant.waitingBody': 'Once the assistant is available, you can start a new conversation. Concepts, features, and scenarios remain available.', 'assistant.needClarification': 'I need clarification', 'assistant.yourAnswer': 'Your answer', 'assistant.continue': 'Continue', 'assistant.askPlaceholder': 'Ask viki…', 'assistant.editPlaceholder': 'Describe what viki should add or change…', 'assistant.listening': 'Listening in Slovak…', 'assistant.shortcut': '⌘⇧M dictates · Enter sends', 'assistant.stop': 'Stop', 'assistant.stopVoice': 'Stop voice input', 'assistant.startVoice': 'Start voice input', 'assistant.voiceTitle': 'Slovak voice input (⌘⇧M / Ctrl+Shift+M)', 'assistant.voiceUnsupported': 'This browser does not support voice input', 'assistant.send': 'Send', 'assistant.conversation': 'Conversation: {title}', 'assistant.conversations': 'Conversations', 'assistant.welcome': 'What do you need to capture?', 'assistant.welcomeBody': 'Ask about company rules or describe a new concept, feature, or scenario.', 'assistant.you': 'You', 'assistant.revision': 'version {id}', 'assistant.exactRevision': 'Exact version: {id}', 'assistant.draftCreated': 'Draft created', 'assistant.disconnected': 'The assistant connection was lost.', 'assistant.reconnecting': 'Reconnecting to the assistant…', 'assistant.reconnect': 'Reconnect', 'assistant.conversationDated': 'Conversation · {date}', 'assistant.conversationNumber': 'Conversation {number}', 'assistant.activity.searching': 'Searching viki…', 'assistant.activity.drafting': 'Creating drafts…', 'assistant.activity.awaiting': 'Drafts awaiting approval…', 'assistant.activity.clarifying': 'Waiting for clarification…', 'assistant.activity.stopping': 'Stopping…', 'assistant.activity.submitting': 'Submitting…', 'assistant.activity.editing': 'Thinking through the edit…', 'assistant.activity.answering': 'Looking for an answer…', 'assistant.loadConversationsFailed': 'Conversations could not be loaded.', 'assistant.loadConversationFailed': 'The conversation could not be loaded.', 'assistant.createConversationFailed': 'A new conversation could not be created.', 'assistant.managementForbidden': 'Assistant management commands are not allowed in viki.', 'assistant.sendFailed': 'The message could not be sent.', 'assistant.stopFailed': 'The assistant could not be stopped.', 'assistant.clarificationFailed': 'The clarification could not be sent.',
  'voice.notAllowed': 'Allow microphone access and try again.', 'voice.unavailable': 'The microphone is unavailable.', 'voice.noSpeech': 'No speech was detected. Try again.', 'voice.unrecognized': 'Voice input could not be recognized.', 'voice.unsupported': 'This browser does not support voice input.', 'voice.startFailed': 'Voice input could not be started.',
}

interface I18nValue {
  locale: Locale
  setLocale: (locale: Locale) => void
  t: Translate
}

const fallback: I18nValue = { locale: 'sk', setLocale: () => undefined, t: (key, values) => interpolate(sk[key], values) }
const I18nContext = createContext<I18nValue>(fallback)

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(readStoredLocale)
  useEffect(() => {
    if (typeof window.localStorage?.setItem === 'function') window.localStorage.setItem('viki.locale', locale)
    document.documentElement.lang = locale
  }, [locale])
  const value = useMemo<I18nValue>(() => ({
    locale,
    setLocale,
    t: (key, values) => interpolate((locale === 'en' ? en : sk)[key], values),
  }), [locale])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

function readStoredLocale(): Locale {
  return typeof window.localStorage?.getItem === 'function' && window.localStorage.getItem('viki.locale') === 'en' ? 'en' : 'sk'
}

export function useI18n(): I18nValue {
  return useContext(I18nContext)
}

export function translate(locale: Locale, key: TranslationKey, values?: Record<string, string | number>): string {
  return interpolate((locale === 'en' ? en : sk)[key], values)
}

function interpolate(value: string, values?: Record<string, string | number>): string {
  if (!values) return value
  return value.replace(/\{([a-zA-Z]+)\}/g, (_, key: string) => String(values[key] ?? `{${key}}`))
}

export function LanguageSwitcher({ className = '' }: { className?: string }) {
  const { locale, setLocale, t } = useI18n()
  const toggle = () => setLocale(locale === 'sk' ? 'en' : 'sk')
  return <div
    className={`language-switcher ${className}`.trim()}
    role="switch"
    tabIndex={0}
    aria-label={t('language.label')}
    aria-checked={locale === 'en'}
    title={locale === 'sk' ? t('language.english') : t('language.slovak')}
    onClick={toggle}
    onKeyDown={(event) => {
      if (event.key !== 'Enter' && event.key !== ' ') return
      event.preventDefault()
      toggle()
    }}
  >
    <span className={locale === 'sk' ? 'active' : ''} aria-hidden="true">SK</span>
    <span className={locale === 'en' ? 'active' : ''} aria-hidden="true">EN</span>
  </div>
}
