/**
 * Stable owner contract for the channel-monitor-user feature.
 *
 * Other features (and their pages) must import shared monitor formatting
 * through this entry instead of reaching into the private presentation layer.
 */
export { useChannelMonitorFormat } from './presentation/composables/useChannelMonitorFormat'
export type { AvailabilityRow } from './presentation/composables/useChannelMonitorFormat'
