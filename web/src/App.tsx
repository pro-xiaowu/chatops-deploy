import {useEffect,useMemo,useState} from 'react'
import {Navigate,Route,Routes,useLocation,useNavigate} from 'react-router-dom'
import {AppstoreOutlined,AuditOutlined,CloudServerOutlined,DashboardOutlined,DeploymentUnitOutlined,KeyOutlined,LoginOutlined,MenuFoldOutlined,MenuUnfoldOutlined,SettingOutlined,TeamOutlined} from '@ant-design/icons'
import {Alert,Avatar,Button,Layout,Menu,Space,Spin,Tag,Tooltip,Typography,message} from 'antd'
import {api} from './api'
import type {Capabilities,User} from './types'
import DashboardPage from './pages/DashboardPage'
import ApplicationsPage from './pages/ApplicationsPage'
import OperationsPage from './pages/OperationsPage'
import ApprovalsPage from './pages/ApprovalsPage'
import ClustersPage from './pages/ClustersPage'
import AccessPage from './pages/AccessPage'
import AuditPage from './pages/AuditPage'
import SettingsPage from './pages/SettingsPage'

const {Header,Sider,Content}=Layout
const items=[['/dashboard',<DashboardOutlined/>,'总览'],['/applications',<AppstoreOutlined/>,'应用'],['/operations',<DeploymentUnitOutlined/>,'发布中心'],['/approvals',<AuditOutlined/>,'审批中心'],['/clusters',<CloudServerOutlined/>,'基础设施'],['/access',<TeamOutlined/>,'访问控制'],['/audit',<KeyOutlined/>,'审计日志'],['/settings',<SettingOutlined/>,'系统设置']] as const

function Console({user}:{user:User}){
  const [collapsed,setCollapsed]=useState(false)
  const location=useLocation()
  const navigate=useNavigate()
  const selected=useMemo(()=>items.find(([path])=>location.pathname.startsWith(path))?.[0]||'/dashboard',[location.pathname])
  return <Layout className="app-shell"><Sider width={224} collapsedWidth={68} collapsed={collapsed} theme="dark" className="sidebar"><div className="brand-mark"><span className="brand-glyph">CD</span>{!collapsed&&<div><strong>ChatOps</strong><small>DEPLOY CONTROL</small></div>}</div><Menu theme="dark" mode="inline" selectedKeys={[selected]} items={items.map(([key,icon,label])=>({key,icon,label}))} onClick={({key})=>navigate(key)}/></Sider><Layout><Header className="topbar"><Button type="text" icon={collapsed?<MenuUnfoldOutlined/>:<MenuFoldOutlined/>} onClick={()=>setCollapsed(value=>!value)} aria-label="切换导航"/><Space size="middle"><Tag color="green">系统正常</Tag><Tooltip title="当前登录用户"><Avatar size="small">{user.display_name.slice(0,1)}</Avatar></Tooltip><Typography.Text>{user.display_name}</Typography.Text></Space></Header><Content className="main-content"><Routes><Route path="/dashboard" element={<DashboardPage/>}/><Route path="/applications" element={<ApplicationsPage/>}/><Route path="/operations" element={<OperationsPage/>}/><Route path="/approvals" element={<ApprovalsPage/>}/><Route path="/clusters" element={<ClustersPage/>}/><Route path="/access" element={<AccessPage/>}/><Route path="/audit" element={<AuditPage/>}/><Route path="/settings" element={<SettingsPage/>}/><Route path="*" element={<Navigate to="/dashboard" replace/>}/></Routes></Content></Layout></Layout>
}

export default function App(){
  const [capabilities,setCapabilities]=useState<Capabilities|null>(null)
  const [user,setUser]=useState<User|null>(null)
  const [loading,setLoading]=useState(true)
  const [loginLoading,setLoginLoading]=useState(false)

  useEffect(()=>{let active=true;api.capabilities().then(value=>{if(active)setCapabilities(value);return api.me()}).then(value=>{if(active)setUser(value)}).catch(()=>undefined).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[])

  const login=async()=>{
    setLoginLoading(true)
    try{await api.devLogin();setUser(await api.me())}catch(error){message.error(error instanceof Error?error.message:'登录失败')}finally{setLoginLoading(false)}
  }

  if(loading)return <main className="auth-screen"><Spin size="large"/></main>
  if(!user)return <main className="auth-screen"><section className="auth-panel"><div className="auth-brand"><span className="brand-glyph">CD</span><div><Typography.Title level={1}>ChatOps Deploy</Typography.Title><Typography.Text type="secondary">发布控制台</Typography.Text></div></div><Space direction="vertical" size="middle" className="auth-actions">{capabilities?.dev_login&&<Button type="primary" size="large" icon={<LoginOutlined/>} loading={loginLoading} onClick={login} block>本地开发登录</Button>}{capabilities?.feishu_login&&<Button size="large" href="/auth/feishu/login" block>飞书登录</Button>}{!capabilities?.dev_login&&!capabilities?.feishu_login&&<Alert type="warning" showIcon message="当前没有可用的登录方式"/>}</Space></section></main>
  return <Console user={user}/>
}
