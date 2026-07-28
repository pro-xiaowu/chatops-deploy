import {useEffect,useState} from 'react'
import {ApiOutlined,CheckCircleOutlined,SaveOutlined} from '@ant-design/icons'
import {Alert,Button,Descriptions,Input,Radio,Space,Tag,Typography,message} from 'antd'
import {api} from '../api'
import type {MessageProvider,MessageProviderSettings,ProviderCapability} from '../types'
import PageHeader from '../components/PageHeader'

const labels:Record<MessageProvider,string>={web:'仅 Web 控制台',feishu:'飞书',wecom:'企业微信',dingtalk:'钉钉'}

export default function SettingsPage(){
  const [settings,setSettings]=useState<MessageProviderSettings|null>(null)
  const [selected,setSelected]=useState<MessageProvider>('web')
  const [saving,setSaving]=useState(false)
  const [checking,setChecking]=useState<MessageProvider|null>(null)

  const load=()=>api.messageProvider().then(value=>{setSettings(value);setSelected(value.provider)}).catch(error=>message.error(error instanceof Error?error.message:'读取消息平台失败'))
  useEffect(()=>{void load()},[])
  const providers=settings?.providers||([] as ProviderCapability[])
  const save=async()=>{setSaving(true);try{await api.selectMessageProvider(selected);message.success('消息平台已更新');await load()}catch(error){message.error(error instanceof Error?error.message:'更新失败')}finally{setSaving(false)}}
  const check=async(provider:MessageProvider)=>{setChecking(provider);try{await api.checkMessageProvider(provider);message.success(`${labels[provider]}连接正常`)}catch(error){message.error(error instanceof Error?error.message:'连接检查失败')}finally{setChecking(null)}}

  return <><PageHeader title="系统设置" description="运行参数和消息平台状态"/><section className="surface provider-settings"><div className="section-heading"><div><Typography.Title level={4}>消息平台</Typography.Title><Typography.Text type="secondary">当前启用：{settings?labels[settings.provider]:'读取中'}</Typography.Text></div><Button type="primary" icon={<SaveOutlined/>} loading={saving} disabled={!settings||selected===settings.provider} onClick={save}>保存</Button></div><Radio.Group className="provider-options" value={selected} onChange={event=>setSelected(event.target.value as MessageProvider)}>{providers.map(provider=><Radio.Button key={provider.provider} value={provider.provider} disabled={!provider.configured}><span>{labels[provider.provider]}</span>{provider.configured?<Tag color={provider.healthy?'green':'warning'}>{provider.healthy?'可用':'待检查'}</Tag>:<Tag>未配置</Tag>}</Radio.Button>)}</Radio.Group>{providers.length===0&&<Alert type="info" showIcon message="正在读取平台配置"/>}<div className="provider-checks">{providers.filter(provider=>provider.provider!=='web').map(provider=><div key={provider.provider}><Space><strong>{labels[provider.provider]}</strong>{provider.configured?<CheckCircleOutlined className="status-ok"/>:<Tag>未配置</Tag>}</Space><Button size="small" icon={<ApiOutlined/>} disabled={!provider.configured} loading={checking===provider.provider} onClick={()=>check(provider.provider)}>测试连接</Button></div>)}</div></section><section className="surface"><Descriptions bordered column={1} size="middle" items={[{key:'worker',label:'执行器模式',children:'all'},{key:'poll',label:'任务轮询',children:'1 秒'},{key:'lease',label:'目标租约',children:'30 秒'}]}/><div className="token-setting"><label>管理 API Token</label><Input.Password placeholder="cdp_..." onChange={event=>localStorage.setItem('chatops_api_token',event.target.value)}/><small>Token 仅保存在当前浏览器。</small></div></section></>
}
