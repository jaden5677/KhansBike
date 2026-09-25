import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router/dom'
import { queryClient } from './api/queryClient'
import { restoreDeviceToken } from './auth/deviceToken'
import { router } from './router'
import './styles/global.css'

// A paired phone keeps its admin sign-in across restarts.
restoreDeviceToken()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
