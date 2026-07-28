import type {Operation} from '../types'
const stages=['申请','审批','排队','部署','完成']
const current:Record<string,number>={pending_approval:1,approved:2,queued:2,running:3,succeeded:4,failed:4,rejected:1,expired:1}
export default function ReleaseTrack({operation}:{operation?:Operation}){const active=operation?current[operation.status]??0:-1;return <div className="release-track">{stages.map((stage,index)=><div className={`track-stage ${index<=active?'is-active':''} ${operation?.status==='failed'&&index===4?'is-failed':''}`} key={stage}><span>{index+1}</span><strong>{stage}</strong>{index<stages.length-1&&<i/>}</div>)}</div>}
