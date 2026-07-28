import {afterEach,describe,expect,it,vi} from 'vitest'
import {api} from './api'

const response=(body:unknown,status=200)=>new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}})

describe('api authentication and message providers',()=>{
  afterEach(()=>vi.restoreAllMocks())

  it('posts development login with credentials and no bearer token',async()=>{
    const request=vi.spyOn(globalThis,'fetch').mockResolvedValue(response({data:{}}))
    await api.devLogin()
    expect(request).toHaveBeenCalledWith('/auth/dev/login',expect.objectContaining({method:'POST',credentials:'include'}))
    expect(JSON.stringify(request.mock.calls[0]?.[1]?.headers)).not.toContain('Bearer')
  })

  it('returns all provider options from capabilities',async()=>{
    vi.spyOn(globalThis,'fetch').mockResolvedValue(response({data:{dev_login:true,feishu_login:false,message_providers:[{provider:'web',configured:true,healthy:true},{provider:'feishu',configured:false,healthy:false},{provider:'wecom',configured:false,healthy:false},{provider:'dingtalk',configured:false,healthy:false}]}}))
    await expect(api.capabilities()).resolves.toMatchObject({dev_login:true,message_providers:expect.arrayContaining([expect.objectContaining({provider:'web'}),expect.objectContaining({provider:'dingtalk'})])})
  })

  it('selects a provider through the authenticated csrf-aware request',async()=>{
    vi.spyOn(globalThis,'fetch').mockResolvedValue(response({data:{provider:'wecom'}}))
    await api.selectMessageProvider('wecom')
    expect(fetch).toHaveBeenCalledWith('/api/v1/settings/message-provider',expect.objectContaining({method:'PUT',credentials:'include'}))
  })
})
