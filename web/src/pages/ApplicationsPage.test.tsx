import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {cleanup,fireEvent,screen,waitFor,within} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {api} from '../api'
import {createTestQueryClient,renderConsole,renderConsoleWithClient} from '../test/render'
import ApplicationsPage from './ApplicationsPage'

vi.mock('../api',()=>({
  api:{
    applications:vi.fn(),
    clusters:vi.fn(),
    environments:vi.fn(),
    createApplication:vi.fn(),
    createEnvironment:vi.fn(),
  },
}))

const application={id:'app-orders',name:'orders-api',description:'Orders',enabled:true}
const billingApplication={id:'app-billing',name:'billing-api',description:'Billing',enabled:true}
const cluster={
  id:'cluster-prod',
  name:'prod',
  api_server:'https://k8s.example',
  credential_version:1,
  enabled:true,
  last_check_status:'healthy',
}

async function openEnvironmentForm(){
  const user=userEvent.setup()
  renderConsole(<ApplicationsPage/>)
  await user.click(await screen.findByRole('button',{name:'管理环境'}))
  await user.click(await screen.findByRole('button',{name:/添加环境/}))
  return user
}

async function fillTestEnvironmentForm(user:ReturnType<typeof userEvent.setup>){
  await user.click(screen.getByRole('combobox',{name:'环境'}))
  await user.click(await screen.findByText('测试'))
  await user.click(screen.getByRole('combobox',{name:'集群'}))
  await user.click(await screen.findByText('prod'))
  fireEvent.change(screen.getByRole('textbox',{name:'Namespace'}),{target:{value:'orders'}})
  fireEvent.change(screen.getByRole('textbox',{name:'Deployment'}),{target:{value:'orders-api'}})
  fireEvent.change(screen.getByRole('textbox',{name:'容器'}),{target:{value:'api'}})
  fireEvent.change(screen.getByRole('textbox',{name:'镜像前缀'}),{
    target:{value:'ghcr.io/acme/orders-api:'},
  })
  fireEvent.change(screen.getByRole('spinbutton',{name:'发布超时'}),{
    target:{value:'480'},
  })
}

describe('ApplicationsPage environment management',()=>{
  beforeEach(()=>{
    vi.mocked(api.applications).mockResolvedValue([application])
    vi.mocked(api.clusters).mockResolvedValue([cluster])
    vi.mocked(api.environments).mockResolvedValue([])
  })

  afterEach(()=>{
    cleanup()
    vi.resetAllMocks()
  })

  it('opens an environment drawer scoped to the selected application',async()=>{
    const user=userEvent.setup()
    renderConsole(<ApplicationsPage/>)

    await user.click(await screen.findByRole('button',{name:'管理环境'}))

    expect(await screen.findByRole('dialog',{name:'orders-api 环境'})).toBeVisible()
    expect(api.environments).toHaveBeenCalledWith('app-orders')
  })

  it('creates an environment with the complete workload mapping',async()=>{
    vi.mocked(api.createEnvironment).mockResolvedValue({
      id:'env-test',
      application_id:'app-orders',
      name:'test',
      cluster_id:'cluster-prod',
      namespace:'orders',
      deployment:'orders-api',
      container:'api',
      image_prefix:'ghcr.io/acme/orders-api:',
      approval_required:false,
      rollout_timeout_seconds:480,
    })
    const user=await openEnvironmentForm()

    await fillTestEnvironmentForm(user)
    await user.click(screen.getByRole('button',{name:'创建环境'}))

    await waitFor(()=>expect(vi.mocked(api.createEnvironment).mock.calls[0]?.[0]).toEqual({
      application_id:'app-orders',
      name:'test',
      cluster_id:'cluster-prod',
      namespace:'orders',
      deployment:'orders-api',
      container:'api',
      image_prefix:'ghcr.io/acme/orders-api:',
      approval_required:false,
      rollout_timeout_seconds:480,
    }))
    await waitFor(()=>expect(api.environments).toHaveBeenCalledTimes(2))
  })

  it('forces approval for production environments',async()=>{
    const user=await openEnvironmentForm()

    await user.click(screen.getByRole('combobox',{name:'环境'}))
    await user.click(await screen.findByText('生产'))

    expect(screen.getByRole('switch',{name:'需要发布审批'})).toBeChecked()
    expect(screen.getByRole('switch',{name:'需要发布审批'})).toBeDisabled()
  })

  it('requires a registered cluster before creating an environment',async()=>{
    vi.mocked(api.clusters).mockResolvedValue([])
    const user=userEvent.setup()
    renderConsole(<ApplicationsPage/>)

    await user.click(await screen.findByRole('button',{name:'管理环境'}))

    expect(await screen.findByText('请先在基础设施中注册 Kubernetes 集群')).toBeVisible()
    expect(screen.getByRole('button',{name:/添加环境/})).toBeDisabled()
  })

  it('keeps the form open and shows the server error when creation fails',async()=>{
    vi.mocked(api.createEnvironment).mockRejectedValue(new Error('集群配置无效'))
    const user=await openEnvironmentForm()
    await fillTestEnvironmentForm(user)

    await user.click(screen.getByRole('button',{name:'创建环境'}))

    expect(await screen.findByText('集群配置无效')).toBeVisible()
    expect(screen.getByLabelText('Namespace')).toHaveValue('orders')
  },10_000)

  it('clears a cancelled environment draft before switching applications',async()=>{
    vi.mocked(api.applications).mockResolvedValue([application,billingApplication])
    const user=userEvent.setup()
    renderConsole(<ApplicationsPage/>)

    const manageButtons=await screen.findAllByRole('button',{name:'管理环境'})
    await user.click(manageButtons[0])
    await user.click(await screen.findByRole('button',{name:/添加环境/}))
    fireEvent.change(screen.getByRole('textbox',{name:'Namespace'}),{
      target:{value:'orders-draft'},
    })
    await user.click(screen.getByRole('button',{name:'Cancel'}))
    const ordersDrawer=screen.getByRole('dialog',{name:'orders-api 环境'})
    await user.click(within(ordersDrawer).getByRole('button',{name:'Close'}))

    await user.click(manageButtons[1])
    await user.click(await screen.findByRole('button',{name:/添加环境/}))

    expect(screen.getByRole('textbox',{name:'Namespace'})).toHaveValue('')
  })

  it('invalidates the Release Center environment cache after creation',async()=>{
    vi.mocked(api.createEnvironment).mockResolvedValue({
      id:'env-test',
      application_id:'app-orders',
      name:'test',
      cluster_id:'cluster-prod',
      namespace:'orders',
      deployment:'orders-api',
      container:'api',
      image_prefix:'ghcr.io/acme/orders-api:',
      approval_required:false,
      rollout_timeout_seconds:480,
    })
    const client=createTestQueryClient()
    client.setQueryData(['envs'],[])
    const user=userEvent.setup()
    renderConsoleWithClient(<ApplicationsPage/>,client)
    await user.click(await screen.findByRole('button',{name:'管理环境'}))
    await user.click(await screen.findByRole('button',{name:/添加环境/}))
    await fillTestEnvironmentForm(user)

    await user.click(screen.getByRole('button',{name:'创建环境'}))

    await waitFor(()=>expect(client.getQueryState(['envs'])?.isInvalidated).toBe(true))
  })

  it('shows environment and cluster load failures with retry actions',async()=>{
    vi.mocked(api.clusters)
      .mockRejectedValueOnce(new Error('没有集群读取权限'))
      .mockResolvedValueOnce([cluster])
    vi.mocked(api.environments)
      .mockRejectedValueOnce(new Error('环境服务暂不可用'))
      .mockResolvedValueOnce([])
    const user=userEvent.setup()
    renderConsole(<ApplicationsPage/>)

    await user.click(await screen.findByRole('button',{name:'管理环境'}))

    expect(await screen.findByText('集群加载失败')).toBeVisible()
    expect(screen.getByText('没有集群读取权限')).toBeVisible()
    expect(screen.getByText('环境加载失败')).toBeVisible()
    expect(screen.getByText('环境服务暂不可用')).toBeVisible()
    await user.click(screen.getByRole('button',{name:/重试集群/}))
    await user.click(screen.getByRole('button',{name:/重试环境/}))
    await waitFor(()=>expect(api.clusters).toHaveBeenCalledTimes(2))
    await waitFor(()=>expect(api.environments).toHaveBeenCalledTimes(2))
  })
})
