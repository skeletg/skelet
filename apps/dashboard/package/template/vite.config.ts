import { defineConfig } from 'vite'
import { skeletDashboard } from '@skelet/dashboard/vite'

// Name your plugins' dashboard packages here too, so dev serves them as source
export default defineConfig(skeletDashboard({ plugins: [] }))
