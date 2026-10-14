<template>
  <form
    novalidate
    data-slot="data-table-toolbar"
    class="flex w-full flex-wrap items-start gap-2 [&_input]:h-8 [&_[data-slot=select-trigger]]:h-8 [&_[data-slot=button]]:h-8"
  >
    <slot />
    <div
      v-if="$slots.filters"
      data-slot="data-table-filters"
      class="ml-auto flex w-full flex-wrap items-start justify-end gap-2 sm:w-auto"
    >
      <slot name="filters" />
    </div>
  </form>
</template>
