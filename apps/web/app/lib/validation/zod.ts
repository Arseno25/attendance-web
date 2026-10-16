import { z } from 'zod'

// Form schemas must work without dynamic code generation under our CSP.
z.config({ jitless: true })
export { z }
