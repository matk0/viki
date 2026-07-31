import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from 'react'
import { api, APIError } from './api/client'
import type { User } from './api/types'

interface AuthValue {
  user: User | null
  loading: boolean
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    api.me()
      .then(({ user }) => setUser(user))
      .catch((error) => {
        if (!(error instanceof APIError) || error.status !== 401) console.error(error)
      })
      .finally(() => setLoading(false))
  }, [])
  const value = useMemo<AuthValue>(() => ({
    user,
    loading,
    login: async (email, password) => {
      const result = await api.login(email, password)
      setUser(result.user)
    },
    logout: async () => {
      await api.logout()
      setUser(null)
    },
  }), [user, loading])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used inside AuthProvider')
  return value
}
