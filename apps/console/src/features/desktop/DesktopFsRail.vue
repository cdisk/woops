<template>
  <div
    class="fs-rail"
    :class="{ open: open }"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
  >
    <div class="handle" :title="t('desktop.controlPanel')">
      <span class="handle-bar" />
    </div>
    <div class="panel">
      <el-button type="primary" size="small" class="exit-btn" @click="emit('exit')">
        {{ t('desktop.exitFullscreen') }}
      </el-button>

      <div class="section-label">{{ t('desktop.shortcuts') }}</div>
      <div class="keys">
        <el-button size="small" :title="t('desktop.sendCtrlAltDel')" @click="emit('special', [KEY.CTRL, KEY.ALT, KEY.DELETE])">
          Ctrl+Alt+Del
        </el-button>
        <el-button size="small" :title="t('desktop.sendWin')" @click="emit('special', [KEY.WIN])">Win</el-button>
        <el-button size="small" :title="t('desktop.sendAltTab')" @click="emit('special', [KEY.ALT, KEY.TAB])">Alt+Tab</el-button>
        <el-button size="small" :title="t('desktop.sendCtrlEsc')" @click="emit('special', [KEY.CTRL, KEY.ESC])">
          Ctrl+Esc
        </el-button>
        <el-button size="small" :title="t('desktop.sendEsc')" @click="emit('special', [KEY.ESC])">Esc</el-button>
      </div>

      <div class="section-label">{{ t('desktop.clipboard') }}</div>
      <el-input
        :model-value="clipboardText"
        type="textarea"
        :rows="6"
        resize="vertical"
        :placeholder="t('desktop.clipboardPlaceholder')"
        @update:model-value="onClipboardInput"
      />
      <div class="clipboard-actions">
        <el-button size="small" type="primary" @click="emit('clipboard-send')">{{ t('desktop.sendToRemote') }}</el-button>
        <el-button size="small" @click="emit('clipboard-read-local')">{{ t('desktop.readLocalClipboard') }}</el-button>
        <el-button size="small" @click="emit('clipboard-copy-local')">{{ t('desktop.copyRemoteToLocal') }}</el-button>
      </div>
      <p class="clipboard-status">{{ clipboardStatus }}</p>
      <p class="clipboard-limit">
        {{ t('desktop.clipboardLimitFs') }}
      </p>
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const KEY = {
  CTRL: 0xFFE3,
  ALT: 0xFFE9,
  WIN: 0xFFEB,
  ESC: 0xFF1B,
  TAB: 0xFF09,
  DELETE: 0xFFFF
}

defineProps({
  clipboardText: { type: String, default: '' },
  clipboardStatus: { type: String, default: '' }
})

const emit = defineEmits([
  'exit',
  'special',
  'update:clipboardText',
  'clipboard-send',
  'clipboard-read-local',
  'clipboard-copy-local'
])

const open = ref(false)
let leaveTimer = 0

function onEnter() {
  window.clearTimeout(leaveTimer)
  open.value = true
}

function onLeave() {
  window.clearTimeout(leaveTimer)
  leaveTimer = window.setTimeout(() => {
    open.value = false
  }, 280)
}

function onClipboardInput(v) {
  emit('update:clipboardText', v)
}

onBeforeUnmount(() => {
  window.clearTimeout(leaveTimer)
})
</script>

<style scoped>
.fs-rail {
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  z-index: 20;
  display: flex;
  align-items: stretch;
  pointer-events: none;
}
.fs-rail > * {
  pointer-events: auto;
}
.handle {
  width: 16px;
  min-height: 72px;
  align-self: center;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(30, 41, 59, 0.72);
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-left: none;
  border-radius: 0 8px 8px 0;
  cursor: pointer;
  transition: background 0.15s ease, opacity 0.15s ease;
}
.handle-bar {
  display: block;
  width: 3px;
  height: 28px;
  border-radius: 2px;
  background: rgba(226, 232, 240, 0.85);
}
.fs-rail.open .handle,
.handle:hover {
  background: rgba(30, 41, 59, 0.95);
}
.panel {
  width: 300px;
  max-height: min(80vh, 640px);
  overflow: auto;
  margin-left: -300px;
  opacity: 0;
  visibility: hidden;
  padding: 12px;
  box-sizing: border-box;
  background: rgba(15, 23, 42, 0.96);
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-left: none;
  border-radius: 0 10px 10px 0;
  box-shadow: 4px 0 24px rgba(0, 0, 0, 0.35);
  color: #e5e7eb;
  transition: margin-left 0.18s ease, opacity 0.18s ease, visibility 0.18s ease;
}
.fs-rail.open .panel {
  margin-left: 0;
  opacity: 1;
  visibility: visible;
}
.exit-btn {
  width: 100%;
  margin-bottom: 12px;
}
.section-label {
  font-size: 12px;
  color: #94a3b8;
  margin: 4px 0 8px;
}
.keys {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}
.clipboard-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.clipboard-status {
  margin: 8px 0 0;
  color: #409eff;
  font-size: 12px;
}
.clipboard-limit {
  margin: 6px 0 0;
  color: #909399;
  font-size: 12px;
  line-height: 1.5;
}
</style>
