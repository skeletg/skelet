import './config'
import './index.css'

// The configuration is set above; the plugins and the dashboard load after it, as imports that can't run first - a
// bundler hoists a statically imported chunk ahead of the code that imports it, so `import { mount }` up there would
// start the dashboard before configure() had run. The plugins register as they're imported, before the dashboard mounts
import('./plugins')
  .then(() => import('@skelet/dashboard'))
  .then(({ mount }) => mount(document.getElementById('root')!))
