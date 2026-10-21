# Contrataque — conventions for building with this design system

Contrataque is a Spanish-language Pokémon battle helper. UI copy is **Spanish**. The look: warm paper ground, near-black ink, one orange accent, hard offset shadows (no blur), pill-shaped controls, Bricolage Grotesque display type over IBM Plex Sans body, IBM Plex Mono for data (types, levels, scores).

## Setup

Always wrap the tree in `ContrataqueProvider`. It supplies the router (`Logo` and `Layout` render `Link`/`NavLink` and throw without it) and the game state `Layout` reads (header pill "Juego: Pokémon X"). `initialPath` sets the active nav item (`/`, `/lideres`, `/equipo`).

```jsx
const { ContrataqueProvider, Layout, Avatar } = window.Contrataque;
<ContrataqueProvider initialPath="/equipo">
  <Layout>{/* page content */}</Layout>
</ContrataqueProvider>
```

`Layout` = full page shell (header with logo + nav, your `children`, `Footer`). For pages without the main nav (e.g. a status page) compose `Logo` + `Footer` yourself.

## Styling idiom: global utility classes + CSS variables

No CSS-in-JS and no style props. Style with the classes in `styles.css` (→ `_ds_bundle.css`) and `var(--*)` tokens.

| Family | Classes |
|---|---|
| Layout | `container` (1240px max, 24px gutter), `main` (page column, 48px top), `stack`, `row`, `row-between`, `wrap`, `grow`, `gap-6/8/10/12` |
| Type | `display`, `title` (page h1), `h-large`, `h-medium`, `h-small`, `h-xsmall`, `lead`, `eyebrow` (mono uppercase accent kicker), `mono-meta`, `mono-tag`, `strong`, `medium`, `small`, `muted`, `warn`, `tiny`, `tiny-12` |
| Surfaces | `card` (white, 1.5px ink border, 20px radius, hard shadow), `cta-band` (orange band), `leader-card` (+ `is-next`), `badge-next` |
| Controls | `btn` + `btn-accent` / `btn-dark` / `btn-light` on `<button>`; `btn-dark` alone on `<a>`; `chips` > `chip` (toggle: set `aria-pressed`, pressed = ink fill); `select-card` (option card, `aria-pressed` = orange); `field` > `label` + `input` |
| Data | `bar` > `bar-fill` (+ `accent`), width inline in %; `mini-team` (list of Pokémon rows); `slot-head` |

Tokens: `--ink` `--paper` `--surface` `--surface-soft` `--surface-muted` `--line` `--line-soft` `--line-card` `--line-input` `--text-body` `--text-muted` `--text-on-dark` `--accent` `--accent-strong` `--accent-soft` `--accent-bright` `--warn` `--ok` `--ok-soft` `--ok-ink` `--font-display` `--font-sans` `--font-mono` `--shadow-hard`. Use tokens for any one-off inline style; never introduce new hex colors.

Rules: selected/pressed state is always `aria-pressed`, never a custom class. Pokémon are shown with `Avatar` (initial in a circle; `className="avatar-accent"` for the highlighted pick). Types are written `"Agua / Siniestro"` in `mono-meta`; levels `"Nv. 48"`. Touch targets ≥44px (all controls already are). No emoji, no gradients.

## Where the truth lives

Read `styles.css` and `_ds_bundle.css` before styling — every class above is defined there. Per-component API and examples: `components/general/<Name>/<Name>.prompt.md` and `<Name>.d.ts`.

## Example

```jsx
<ContrataqueProvider initialPath="/equipo">
  <Layout>
    <main className="container main">
      <p className="eyebrow">Un equipo por partida</p>
      <h1 className="title">Mi equipo</h1>
      <section className="card stack" style={{ padding: 24, gap: 20, maxWidth: 520 }}>
        <div className="row-between wrap">
          <h2 className="h-medium">Equipo de Pokémon X</h2>
          <button type="button" className="btn btn-accent">Guardar equipo</button>
        </div>
        <div className="slot-head">
          <Avatar name="Greninja" size={48} />
          <span className="stack grow">
            <span className="strong">Greninja</span>
            <span className="mono-meta tiny-12">Agua / Siniestro</span>
          </span>
          <span className="mono-meta">Nv. 48</span>
        </div>
      </section>
    </main>
  </Layout>
</ContrataqueProvider>
```

# Contrataque (poketeamcalculatorfrontend@0.0.0)

This design system is the published poketeamcalculatorfrontend React library, bundled as a single
browser global. All 4 components are the real upstream code.

## Where things are

- `_ds_bundle.js` — the whole-DS bundle at the project root; loads every component to `window.Contrataque`. First line is a `/* @ds-bundle: … */` metadata header.
- `styles.css` — the single stylesheet entry: it `@import`s the tokens, fonts, and component styles (`_ds_bundle.css`). Link this one file.
- `components/<group>/<Name>/<Name>.prompt.md` (example JSX + variants), `<Name>.d.ts` (types), `<Name>.html` (variant grid).
- `tokens/*.css` — CSS custom properties, names verbatim from upstream.
- `fonts/` — `@font-face` files + `fonts.css` (when the package ships fonts).

For a specific component, `read_file("components/<group>/<Name>/<Name>.prompt.md")`.

## Loading

Add these two lines to your page once (React must be on the page first):

```html
<link rel="stylesheet" href="styles.css">
<script src="_ds_bundle.js"></script>
```

Components are then available at `window.Contrataque.*`. Mount into a dedicated child node (e.g. `<div id="ds-root">`), not the host page's own React root, so the two trees don't collide:

```jsx
const { Avatar } = window.Contrataque;
ReactDOM.createRoot(document.getElementById('ds-root')).render(<Avatar />);
```

Wrap the tree in the provider — most components read theme/i18n from context:

```jsx
<ContrataqueProvider>{children}</ContrataqueProvider>
```

## Tokens

24 CSS custom properties from poketeamcalculatorfrontend. Names are
preserved verbatim from upstream. They are declared inside `_ds_bundle.css` (this DS ships one compiled stylesheet rather than separate token files).

- **color** (6): `--surface`, `--surface-soft`, `--surface-muted`, …
- **typography** (3): `--font-display`, `--font-sans`, `--font-mono`
- **shadow** (1): `--shadow-hard`
- **other** (14): `--ink`, `--paper`, `--line`, …

## Components

### general
- `Avatar`
- `Footer`
- `Layout`
- `Logo`
