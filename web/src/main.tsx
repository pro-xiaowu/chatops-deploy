import React from 'react'
import ReactDOM from 'react-dom/client'
import {QueryClient,QueryClientProvider} from '@tanstack/react-query'
import {App as AntdApp,ConfigProvider} from 'antd'
import {BrowserRouter} from 'react-router-dom'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import './styles.css'

const client=new QueryClient({defaultOptions:{queries:{retry:1,staleTime:10_000,refetchOnWindowFocus:false}}})
ReactDOM.createRoot(document.getElementById('root')!).render(<React.StrictMode><ConfigProvider locale={zhCN} theme={{token:{colorPrimary:'#087f5b',borderRadius:6,fontFamily:'Inter, "Noto Sans SC", system-ui, sans-serif',colorBgLayout:'#f3f5f4'}}}><AntdApp><QueryClientProvider client={client}><BrowserRouter><App/></BrowserRouter></QueryClientProvider></AntdApp></ConfigProvider></React.StrictMode>)
