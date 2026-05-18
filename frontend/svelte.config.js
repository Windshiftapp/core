import { preprocessMeltUI, sequence } from '@melt-ui/pp';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// preprocessMeltUI calls the Svelte parser on every .svelte file it sees,
// even files that don't use melt-ui — that AST parse is what dominates
// vite-plugin-svelte:load-custom in our build report (85% of plugin time
// across 498 files). Only ~14 files actually use melt-ui, so we wrap the
// preprocessor with a regex fast-path that returns early for files with
// no melt markers. The regex covers both the action attribute syntax
// (`use:melt`, `melt={...}`) and the package import.
const meltMarkerRegex = /\buse:melt\b|\bmelt\s*=\s*\{|@melt-ui/;

function fastMeltGuard(melt) {
  return {
    name: 'melt-ui-fast-guard',
    async markup(input) {
      if (!meltMarkerRegex.test(input.content)) {
        return null;
      }
      return melt.markup(input);
    },
  };
}

export default {
  preprocess: sequence([
    vitePreprocess(), // Must come first - handles TypeScript, PostCSS, etc.
    fastMeltGuard(preprocessMeltUI()), // Must come last per @melt-ui/pp docs
  ]),
};
