import {QueryClient,QueryClientProvider} from '@tanstack/react-query'
import {render,type RenderResult} from '@testing-library/react'
import {App as AntdApp} from 'antd'
import type {ReactElement} from 'react'
import {MemoryRouter} from 'react-router-dom'

export function renderConsole(ui:ReactElement):RenderResult {
  const client=new QueryClient({
    defaultOptions:{queries:{retry:false},mutations:{retry:false}},
  })
  return render(
    <AntdApp>
      <QueryClientProvider client={client}>
        <MemoryRouter>{ui}</MemoryRouter>
      </QueryClientProvider>
    </AntdApp>,
  )
}
