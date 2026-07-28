import {Tag} from 'antd'
const colors:Record<string,string>={succeeded:'success',running:'processing',queued:'blue',approved:'cyan',pending_approval:'gold',failed:'error',rejected:'default',expired:'default'}
const labels:Record<string,string>={succeeded:'成功',running:'执行中',queued:'排队中',approved:'已批准',pending_approval:'待审批',failed:'失败',rejected:'已拒绝',expired:'已过期'}
export default function StatusTag({status}:{status:string}){return <Tag color={colors[status]||'default'}>{labels[status]||status}</Tag>}
