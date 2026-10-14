export interface DataTableColumn {
  id: string
  label: string
  headClass?: string
  cellClass?: string
  hidden?: boolean
}

export interface TableAction {
  label: string
  icon:
    | 'view'
    | 'edit'
    | 'face'
    | 'revoke'
    | 'pair'
    | 'approve'
    | 'reject'
    | 'record'
    | 'checkout'
    | 'correction'
  to?: string
  onSelect?: () => void
  destructive?: boolean
  hidden?: boolean
  disabled?: boolean
}
