import type {ReactNode} from 'react'
import {Typography} from 'antd'
export default function PageHeader({title,description,actions}:{title:string;description:string;actions?:ReactNode}){return <div className="page-header"><div><Typography.Title level={2}>{title}</Typography.Title><Typography.Text type="secondary">{description}</Typography.Text></div>{actions&&<div>{actions}</div>}</div>}
