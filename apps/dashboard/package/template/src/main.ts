import './config'
import './index.css'

// The configuration is set above, and the dashboard loads after it as an import that can't run first - a bundler
// hoists a statically imported chunk ahead of the code that imports it, so `import { mount }` up there would start
// the dashboard before configure() had run, and it refuses to start unconfigured. mount loads the plugins itself,
// once the dashboard's own menu is there for them to add to
void import('@skelet/dashboard').then(({ mount }) => mount(document.getElementById('root')!, { plugins: () => import('./plugins') }))
