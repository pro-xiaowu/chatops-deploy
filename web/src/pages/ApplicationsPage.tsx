import {useState} from 'react'
import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query'
import {Alert,App as AntdApp,Button,Drawer,Form,Input,InputNumber,Modal,Select,Switch,Table,Tag} from 'antd'
import {PlusOutlined,ReloadOutlined} from '@ant-design/icons'
import {api} from '../api'
import PageHeader from '../components/PageHeader'
import type {Application} from '../types'

export default function ApplicationsPage(){
  const {message}=AntdApp.useApp()
  const [applicationModalOpen,setApplicationModalOpen]=useState(false)
  const [environmentModalOpen,setEnvironmentModalOpen]=useState(false)
  const [selectedApplication,setSelectedApplication]=useState<Application|null>(null)
  const [applicationForm]=Form.useForm()
  const [environmentForm]=Form.useForm()
  const environmentName=Form.useWatch('name',environmentForm)
  const client=useQueryClient()
  const applications=useQuery({queryKey:['apps'],queryFn:api.applications})
  const clusters=useQuery({
    queryKey:['clusters'],
    queryFn:api.clusters,
    enabled:Boolean(selectedApplication),
  })
  const environments=useQuery({
    queryKey:['environments',selectedApplication?.id],
    queryFn:()=>api.environments(selectedApplication!.id),
    enabled:Boolean(selectedApplication),
  })
  const createApplication=useMutation({
    mutationFn:api.createApplication,
    onSuccess:()=>{
      message.success('应用已创建')
      setApplicationModalOpen(false)
      applicationForm.resetFields()
      client.invalidateQueries({queryKey:['apps']})
    },
  })
  const createEnvironment=useMutation({
    mutationFn:api.createEnvironment,
    onSuccess:()=>{
      message.success('环境已创建')
      setEnvironmentModalOpen(false)
      environmentForm.resetFields()
      client.invalidateQueries({queryKey:['environments',selectedApplication?.id]})
      client.invalidateQueries({queryKey:['envs']})
    },
    onError:error=>message.error(error instanceof Error?error.message:'创建环境失败'),
  })

  const clusterName=(clusterID:string)=>
    clusters.data?.find(cluster=>cluster.id===clusterID)?.name||clusterID
  const closeEnvironmentModal=()=>{
    setEnvironmentModalOpen(false)
    environmentForm.resetFields()
  }

  return <>
    <PageHeader
      title="应用目录"
      description="只允许对预注册工作负载执行变更"
      actions={
        <Button
          type="primary"
          icon={<PlusOutlined/>}
          onClick={()=>setApplicationModalOpen(true)}
        >
          添加应用
        </Button>
      }
    />
    <section className="surface">
      <Table
        rowKey="id"
        loading={applications.isLoading}
        dataSource={applications.data}
        columns={[
          {title:'应用',dataIndex:'name'},
          {title:'说明',dataIndex:'description'},
          {
            title:'状态',
            dataIndex:'enabled',
            render:value=><Tag color={value?'green':'default'}>{value?'启用':'停用'}</Tag>,
          },
          {title:'标识',dataIndex:'id',className:'mono'},
          {
            title:'操作',
            key:'actions',
            render:(_,application:Application)=>
              <Button type="link" onClick={()=>setSelectedApplication(application)}>管理环境</Button>,
          },
        ]}
      />
    </section>

    <Modal
      title="添加应用"
      open={applicationModalOpen}
      onCancel={()=>setApplicationModalOpen(false)}
      onOk={()=>applicationForm.submit()}
      okText="创建应用"
      confirmLoading={createApplication.isPending}
    >
      <Form
        form={applicationForm}
        layout="vertical"
        onFinish={values=>createApplication.mutate(values)}
      >
        <Form.Item label="应用名称" name="name" rules={[{required:true}]}>
          <Input placeholder="orders-api"/>
        </Form.Item>
        <Form.Item label="说明" name="description">
          <Input.TextArea rows={3}/>
        </Form.Item>
      </Form>
    </Modal>

    <Drawer
      title={selectedApplication?selectedApplication.name+' 环境':'应用环境'}
      width="min(920px, 100vw)"
      open={Boolean(selectedApplication)}
      onClose={()=>{
        closeEnvironmentModal()
        setSelectedApplication(null)
      }}
      extra={
        <Button
          type="primary"
          icon={<PlusOutlined/>}
          onClick={()=>setEnvironmentModalOpen(true)}
          disabled={!clusters.data?.length}
        >
          添加环境
        </Button>
      }
    >
      {clusters.isError&&
        <Alert
          type="error"
          showIcon
          message="集群加载失败"
          description={clusters.error instanceof Error?clusters.error.message:'无法读取集群'}
          action={
            <Button size="small" icon={<ReloadOutlined/>} onClick={()=>clusters.refetch()}>
              重试集群
            </Button>
          }
          style={{marginBottom:16}}
        />
      }
      {environments.isError&&
        <Alert
          type="error"
          showIcon
          message="环境加载失败"
          description={environments.error instanceof Error?environments.error.message:'无法读取环境'}
          action={
            <Button size="small" icon={<ReloadOutlined/>} onClick={()=>environments.refetch()}>
              重试环境
            </Button>
          }
          style={{marginBottom:16}}
        />
      }
      {!clusters.isLoading&&!clusters.isError&&clusters.data?.length===0&&
        <Alert
          type="warning"
          showIcon
          message="请先在基础设施中注册 Kubernetes 集群"
          style={{marginBottom:16}}
        />
      }
      <Table
        rowKey="id"
        size="small"
        pagination={false}
        loading={environments.isLoading||clusters.isLoading}
        dataSource={environments.data}
        scroll={{x:860}}
        columns={[
          {title:'环境',dataIndex:'name'},
          {
            title:'集群',
            dataIndex:'cluster_id',
            render:value=>clusterName(value),
          },
          {title:'Namespace',dataIndex:'namespace',className:'mono'},
          {title:'Deployment',dataIndex:'deployment',className:'mono'},
          {title:'容器',dataIndex:'container',className:'mono'},
          {title:'镜像前缀',dataIndex:'image_prefix',className:'mono'},
          {
            title:'审批',
            dataIndex:'approval_required',
            render:value=><Tag color={value?'gold':'default'}>{value?'需要':'不需要'}</Tag>,
          },
          {
            title:'超时',
            dataIndex:'rollout_timeout_seconds',
            render:value=>value+' 秒',
          },
        ]}
      />
    </Drawer>

    <Modal
      title={selectedApplication?'为 '+selectedApplication.name+' 添加环境':'添加环境'}
      open={environmentModalOpen}
      onCancel={closeEnvironmentModal}
      onOk={()=>environmentForm.submit()}
      okText="创建环境"
      confirmLoading={createEnvironment.isPending}
      destroyOnHidden
    >
      <Form
        form={environmentForm}
        layout="vertical"
        initialValues={{
          name:'development',
          approval_required:false,
          rollout_timeout_seconds:600,
        }}
        onValuesChange={changed=>{
          if(changed.name==='production'){
            environmentForm.setFieldValue('approval_required',true)
          }
        }}
        onFinish={values=>{
          if(!selectedApplication)return
          createEnvironment.mutate({...values,application_id:selectedApplication.id})
        }}
      >
        <Form.Item label="环境" name="name" rules={[{required:true}]}>
          <Select options={[
            {value:'development',label:'开发'},
            {value:'test',label:'测试'},
            {value:'production',label:'生产'},
          ]}/>
        </Form.Item>
        <Form.Item label="集群" name="cluster_id" rules={[{required:true}]}>
          <Select
            options={clusters.data?.map(cluster=>({
              value:cluster.id,
              label:cluster.name,
            }))}
          />
        </Form.Item>
        <Form.Item label="Namespace" name="namespace" rules={[{required:true}]}>
          <Input placeholder="orders"/>
        </Form.Item>
        <Form.Item label="Deployment" name="deployment" rules={[{required:true}]}>
          <Input placeholder="orders-api"/>
        </Form.Item>
        <Form.Item label="容器" name="container" rules={[{required:true}]}>
          <Input placeholder="api"/>
        </Form.Item>
        <Form.Item label="镜像前缀" name="image_prefix" rules={[{required:true}]}>
          <Input placeholder="ghcr.io/acme/orders-api:"/>
        </Form.Item>
        <Form.Item
          label="发布超时"
          name="rollout_timeout_seconds"
          rules={[{required:true}]}
        >
          <InputNumber min={30} max={3600} step={30} style={{width:'100%'}}/>
        </Form.Item>
        <Form.Item
          label="需要发布审批"
          name="approval_required"
          valuePropName="checked"
        >
          <Switch disabled={environmentName==='production'}/>
        </Form.Item>
      </Form>
    </Modal>
  </>
}
