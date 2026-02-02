import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'
import { preprocessMeltUI, sequence } from '@melt-ui/pp'

export default {
  // Consult https://svelte.dev/docs#compile-time-svelte-preprocess
  // for more information about preprocessors
  preprocess: sequence([
    vitePreprocess(),    // Must come first - handles TypeScript, PostCSS, etc.
    preprocessMeltUI()   // Must come last per @melt-ui/pp docs
  ]),
}
