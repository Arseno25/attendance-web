// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'

export default withNuxt(
  {
    files: ['app/components/ui/**/*.vue'],
    // Registry primitives intentionally forward optional undefined props.
    rules: { 'vue/require-default-prop': 'off' },
  },
)
