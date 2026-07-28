import {useMemo,useState} from 'react'
import {Navigate,Route,Routes,useLocation,useNavigate} from 'react-router-dom'
import {AppstoreOutlined,AuditOutlined,CloudServerOutlined,DashboardOutlined,DeploymentUnitOutlined,KeyOutlined,MenuFoldOutlined,MenuUnfoldOutlined,SettingOutlined,TeamOutlined} from '@ant-design/icons'
import {Avatar,Button,Layout,Menu,Space,Tag,Tooltip,Typography} from 'antd'
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
export default function App(){const [collapsed,setCollapsed]=useState(false);const location=useLocation();const navigate=useNavigate();const selected=useMemo(()=>items.find(([p])=>location.pathname.startsWith(p))?.[0]||'/dashboard',[location.pathname]);return <Layout className="app-shell"><Sider width={224} collapsedWidth={68} collapsed={collapsed} theme="dark" className="sidebar"><div className="brand-mark"><span className="brand-glyph">CD</span>{!collapsed&&<div><strong>ChatOps</strong><small>DEPLOY CONTROL</small></div>}</div><Menu theme="dark" mode="inline" selectedKeys={[selected]} items={items.map(([key,icon,label])=>({key,icon,label}))} onClick={({key})=>navigate(key)}/></Sider><Layout><Header className="topbar"><Button type="text" icon={collapsed?<MenuUnfoldOutlined/>:<MenuFoldOutlined/>} onClick={()=>setCollapsed(v=>!v)} aria-label="切换导航"/><Space size="middle"><Tag color="green">系统正常</Tag><Tooltip title="当前登录用户"><Avatar size="small">管</Avatar></Tooltip><Typography.Text>管理员</Typography.Text></Space></Header><Content className="main-content"><Routes><Route path="/dashboard" element={<DashboardPage/>}/><Route path="/applications" element={<ApplicationsPage/>}/><Route path="/operations" element={<OperationsPage/>}/><Route path="/approvals" element={<ApprovalsPage/>}/><Route path="/clusters" element={<ClustersPage/>}/><Route path="/access" element={<AccessPage/>}/><Route path="/audit" element={<AuditPage/>}/><Route path="/settings" element={<SettingsPage/>}/><Route path="*" element={<Navigate to="/dashboard" replace/>}/></Routes></Content></Layout></Layout>}
