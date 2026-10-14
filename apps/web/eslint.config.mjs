// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'

export default withNuxt(
  {
    files: ['app/**/*.vue'],
    // Match Prettier's Vue formatting for void HTML elements.
    rules: { 'vue/html-self-closing': ['warn', { html: { void: 'always' } }] },
  },
  {
    files: ['app/components/ui/**/*.vue'],
    // Registry primitives intentionally forward optional undefined props.
    rules: {
      'vue/require-default-prop': 'off',
      'vue/html-self-closing': ['warn', { html: { void: 'any' } }],
    },
  },
)
