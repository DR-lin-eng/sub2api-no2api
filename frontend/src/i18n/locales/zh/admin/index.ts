import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import promptAudit from './promptAudit'
import cluster from './cluster'
import ingressRisk from './ingressRisk'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...promptAudit,
  ...cluster,
  ...ingressRisk,
}
