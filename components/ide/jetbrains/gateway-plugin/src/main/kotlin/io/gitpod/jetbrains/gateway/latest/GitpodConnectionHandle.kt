// Copyright (c) 2023 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License.AGPL.txt in the project root for license information.

package io.gitpod.jetbrains.gateway.latest

import com.jetbrains.gateway.api.CustomConnectionFrameComponentProvider
import com.jetbrains.gateway.api.CustomConnectionFrameContext
import com.jetbrains.gateway.api.GatewayConnectionHandle
import com.jetbrains.rd.util.lifetime.Lifetime
import io.gitpod.jetbrains.gateway.GitpodConnectionProvider.ConnectParams
import io.gitpod.jetbrains.gateway.common.GitpodConnectionHandleFactory
import javax.swing.JComponent

class GitpodConnectionHandle(
        lifetime: Lifetime,
        private val component: JComponent,
        private val params: ConnectParams
) : GatewayConnectionHandle(lifetime) {
    override fun customComponentProvider(lifetime: Lifetime) = object : CustomConnectionFrameComponentProvider {
        override val closeConfirmationText = "Disconnect from ${getTitle()}?"
        override fun createComponent(context: CustomConnectionFrameContext) = component
    }

    override fun getTitle(): String {
        return params.title
    }

    override fun hideToTrayOnStart(): Boolean {
        return false
    }
}

class LatestGitpodConnectionHandleFactory : GitpodConnectionHandleFactory {
    override fun createGitpodConnectionHandle(
            lifetime: Lifetime,
            component: JComponent,
            params: ConnectParams
    ): GatewayConnectionHandle {
        return GitpodConnectionHandle(lifetime, component, params)
    }
}
