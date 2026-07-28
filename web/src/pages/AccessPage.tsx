import {useState} from 'react'
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query'
import {Button,Form,Input,Modal,Select,Space,Table,Tag,message} from 'antd'
import {LinkOutlined,UserAddOutlined} from '@ant-design/icons'
import {api} from '../api'
import type {MessageProvider} from '../types'
import PageHeader from '../components/PageHeader'

const labels:Record<MessageProvider,string>={web:'Web 控制台',feishu:'飞书',wecom:'企业微信',dingtalk:'钉钉'}

export default function AccessPage(){
  const [userOpen,setUserOpen]=useState(false)
  const [identityUser,setIdentityUser]=useState<{id:string;name:string}|null>(null)
  const [userForm]=Form.useForm()
  const [identityForm]=Form.useForm()
  const client=useQueryClient()
  const users=useQuery({queryKey:['users'],queryFn:api.users})
  const identities=useQuery({queryKey:['identities',identityUser?.id],queryFn:()=>api.identities(identityUser!.id),enabled:Boolean(identityUser)})
  const createUser=useMutation({mutationFn:api.createUser,onSuccess:()=>{message.success('用户已添加');setUserOpen(false);userForm.resetFields();client.invalidateQueries({queryKey:['users']})},onError:error=>message.error(error instanceof Error?error.message:'添加用户失败')})
  const createIdentity=useMutation({mutationFn:(input:unknown)=>api.createIdentity(identityUser!.id,input),onSuccess:()=>{message.success('身份已绑定');identityForm.resetFields();client.invalidateQueries({queryKey:['identities',identityUser?.id]})},onError:error=>message.error(error instanceof Error?error.message:'绑定身份失败')})

  return <>
    <PageHeader title="访问控制" description="管理本地用户及其消息平台身份" actions={<Button type="primary" icon={<UserAddOutlined/>} onClick={()=>setUserOpen(true)}>添加用户</Button>}/>
    <section className="surface"><Table rowKey="id" loading={users.isLoading} dataSource={users.data||[]} columns={[
      {title:'用户',dataIndex:'display_name'},
      {title:'状态',dataIndex:'enabled',render:(value:boolean)=><Tag color={value?'green':'default'}>{value?'启用':'停用'}</Tag>},
      {title:'用户 ID',dataIndex:'id',className:'mono'},
      {title:'身份',key:'identity',render:(_:unknown,row:{id:string;display_name:string})=><Button size="small" icon={<LinkOutlined/>} onClick={()=>setIdentityUser({id:row.id,name:row.display_name})}>绑定身份</Button>},
    ]}/></section>
    <Modal title="添加用户" open={userOpen} onCancel={()=>setUserOpen(false)} onOk={()=>userForm.submit()}>
      <Form form={userForm} layout="vertical" onFinish={value=>createUser.mutate(value)}><Form.Item name="display_name" label="显示名称" rules={[{required:true}]}><Input/></Form.Item></Form>
    </Modal>
    <Modal title={identityUser?`绑定身份：${identityUser.name}`:'绑定身份'} open={Boolean(identityUser)} onCancel={()=>setIdentityUser(null)} onOk={()=>identityForm.submit()}>
      <Space direction="vertical" style={{width:'100%'}}>
        <Table size="small" pagination={false} loading={identities.isLoading} dataSource={identities.data||[]} rowKey="id" columns={[{title:'平台',dataIndex:'provider',render:(value:MessageProvider)=>labels[value]||value},{title:'Subject ID',dataIndex:'subject_id',className:'mono'},{title:'显示名称',dataIndex:'display_name'}]}/>
        <Form form={identityForm} layout="vertical" onFinish={value=>createIdentity.mutate(value)}><Form.Item name="provider" label="平台" rules={[{required:true}]}><Select options={Object.entries(labels).map(([value,label])=>({value,label}))}/></Form.Item><Form.Item name="subject_id" label="Subject ID" rules={[{required:true}]}><Input placeholder="平台中的用户唯一标识"/></Form.Item><Form.Item name="display_name" label="显示名称"><Input/></Form.Item></Form>
      </Space>
    </Modal>
  </>
}
